import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Events } from '@wailsio/runtime'
import * as PackageService from '../bindings/depmanager/packages/packageservice'
import AppUpdates from './AppUpdates'

type GlobalPackage = {
  name: string
  version: string
}

type PackageManagerInfo = {
  name: string
  version: string
  installed: boolean
  packages: GlobalPackage[]
  error?: string
}

type Notice = {
  kind: 'success' | 'error'
  message: string
  output?: string
}

type PackageAction = 'update' | 'remove'
type ActiveAction = { action: PackageAction, stage: string }
type PackageProgress = { manager: string, packageName: string, action: PackageAction, stage: string }

const managerNames = ['npm', 'pnpm', 'bun']
const latestVersionConcurrency = 8

function usesPnpmSelfUpdate(manager: string, packageName: string) {
  return manager === 'pnpm' && (packageName === 'pnpm' || packageName === '@pnpm/exe')
}

function normalizeVersion(version: string) {
  return version.trim().replace(/^v(?=\d)/i, '')
}

function Icon({ name }: { name: 'search' | 'refresh' | 'package' | 'trash' | 'check' | 'alert' }) {
  const common = {
    width: 17,
    height: 17,
    viewBox: '0 0 24 24',
    fill: 'none',
    stroke: 'currentColor',
    strokeWidth: 1.8,
    strokeLinecap: 'round' as const,
    strokeLinejoin: 'round' as const,
    'aria-hidden': true as const,
  }
  const paths = {
    search: <><circle cx="11" cy="11" r="7" /><path d="m20 20-4-4" /></>,
    refresh: <><path d="M20 7v5h-5" /><path d="M20 12a8 8 0 0 0-14.8-4M4 17v-5h5" /><path d="M4 12a8 8 0 0 0 14.8 4" /></>,
    package: <><path d="m12 3 8 4.5v9L12 21l-8-4.5v-9L12 3Z" /><path d="m4.3 7.7 7.7 4.5 7.7-4.5M12 12.2V21" /></>,
    trash: <><path d="M3 6h18M8 6V4h8v2m3 0-1 14H6L5 6" /><path d="M10 11v5m4-5v5" /></>,
    check: <path d="m5 12 4 4L19 6" />,
    alert: <><path d="M12 9v4m0 4h.01" /><path d="M10.3 3.9 2.7 17a2 2 0 0 0 1.7 3h15.2a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z" /></>,
  }
  return <svg {...common}>{paths[name]}</svg>
}

function App() {
  const [managers, setManagers] = useState<PackageManagerInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [appUpdating, setAppUpdating] = useState(false)
  const [search, setSearch] = useState('')
  const [activeManager, setActiveManager] = useState('all')
  const [activeActions, setActiveActions] = useState<Record<string, ActiveAction>>({})
  const [latestVersions, setLatestVersions] = useState<Record<string, string | null | undefined>>({})
  const [notice, setNotice] = useState<Notice | null>(null)
  const latestCheckRun = useRef(0)
  const managerLoadRun = useRef(0)

  useEffect(() => Events.On('package:progress', (event) => {
    const progress = event.data as PackageProgress
    const key = `${progress.manager}:${progress.packageName}`
    setActiveActions((current) => {
      const active = current[key]
      if (!active || active.action !== progress.action || active.stage === 'Refreshing') return current
      return { ...current, [key]: { ...active, stage: progress.stage } }
    })
  }), [])

  const checkLatestVersions = useCallback(async (data: PackageManagerInfo[]) => {
    const packages = data.flatMap((manager) => manager.installed
      ? manager.packages.map((pkg) => ({ manager: manager.name, ...pkg, key: `${manager.name}:${pkg.name}` }))
      : [])
    const run = ++latestCheckRun.current

    setLatestVersions((current) => {
      const next = { ...current }
      for (const pkg of packages) next[pkg.key] = undefined
      return next
    })

    let nextPackage = 0
    const checkWorker = async () => {
      while (latestCheckRun.current === run && nextPackage < packages.length) {
        const pkg = packages[nextPackage++]
        try {
          const version = await PackageService.GetLatestPackageVersion(pkg.manager, pkg.name)
          if (latestCheckRun.current !== run) return
          setLatestVersions((current) => ({ ...current, [pkg.key]: version || null }))
        } catch {
          if (latestCheckRun.current !== run) return
          setLatestVersions((current) => ({ ...current, [pkg.key]: null }))
        }
      }
    }

    await Promise.all(Array.from({ length: Math.min(latestVersionConcurrency, packages.length) }, () => checkWorker()))
  }, [])

  const loadManagers = useCallback(async (showLoading = false) => {
    const run = ++managerLoadRun.current
    if (showLoading) setLoading(true)
    setRefreshing(true)
    try {
      const data = await PackageService.GetPackageManagers()
      if (managerLoadRun.current !== run) return
      const normalized = (data ?? []).map((manager) => ({ ...manager, packages: manager.packages ?? [] }))
      setManagers(normalized)
      void checkLatestVersions(normalized)
    } catch (error) {
      if (managerLoadRun.current !== run) return
      setNotice({ kind: 'error', message: error instanceof Error ? error.message : 'Could not read package managers.' })
    } finally {
      if (managerLoadRun.current === run) {
        setLoading(false)
        setRefreshing(false)
      }
    }
  }, [checkLatestVersions])

  useEffect(() => {
    void loadManagers(true)
    return () => {
      ++managerLoadRun.current
      ++latestCheckRun.current
    }
  }, [loadManagers])

  const installedManagers = useMemo(() => managers.filter((manager) => manager.installed), [managers])
  const allPackages = useMemo(() => installedManagers.flatMap((manager) => manager.packages.map((pkg) => ({ ...pkg, manager: manager.name }))), [installedManagers])
  const filteredPackages = useMemo(() => {
    const query = search.trim().toLowerCase()
    return allPackages.filter((pkg) => (activeManager === 'all' || pkg.manager === activeManager) && (!query || `${pkg.name} ${pkg.version} ${pkg.manager}`.toLowerCase().includes(query)))
  }, [activeManager, allPackages, search])
  const pnpmBusy = Object.keys(activeActions).some((key) => key.startsWith('pnpm:'))
  const pnpmSelfUpdating = ['pnpm:pnpm', 'pnpm:@pnpm/exe'].some((key) => activeActions[key]?.action === 'update')
  const runAction = async (manager: string, pkg: GlobalPackage, action: PackageAction) => {
    if (appUpdating) return
    const selfUpdate = usesPnpmSelfUpdate(manager, pkg.name)
    if (manager === 'pnpm' && (pnpmSelfUpdating || (action === 'update' && selfUpdate && pnpmBusy))) return
    if (action === 'update' && selfUpdate && !window.confirm(
      `${pkg.name} is the pnpm package manager itself.\n\nUpdating it will run:\npnpm self-update\n\nRun this command?`,
    )) return
    if (action === 'remove' && !window.confirm(`Remove ${pkg.name} from ${manager}'s global packages?`)) return
    const key = `${manager}:${pkg.name}`
    setActiveActions((current) => ({ ...current, [key]: { action, stage: 'Preparing' } }))
    setNotice(null)
    try {
      const result = await PackageService.RunPackageAction(manager, pkg.name, action)
      if (!result.success) {
        setNotice({ kind: 'error', message: result.message, output: result.output })
      } else {
        setNotice({ kind: 'success', message: result.message, output: result.output })
        setActiveActions((current) => ({ ...current, [key]: { action, stage: 'Refreshing' } }))
        await loadManagers()
      }
    } catch (error) {
      setNotice({ kind: 'error', message: error instanceof Error ? error.message : `Could not ${action} ${pkg.name}.` })
    } finally {
      setActiveActions((current) => {
        const next = { ...current }
        delete next[key]
        return next
      })
    }
  }

  return (
    <main className="app-shell">
      <div className="page-content">
        <section className="filter-section" aria-label="Filter packages by manager">
          <span className="filter-label">Package manager</span>
          <div className="manager-filters">
            <button className={`manager-filter ${activeManager === 'all' ? 'active' : ''}`} onClick={() => setActiveManager('all')}>
              <span>All packages</span><span className="filter-count">{allPackages.length}</span>
            </button>
            {managerNames.map((name) => {
              const manager = managers.find((item) => item.name === name)
              const installed = manager?.installed ?? false
              const packageCount = manager?.packages.length ?? 0
              const label = name === 'bun' ? 'Bun' : name
              return (
                <button
                  key={name}
                  className={`manager-filter ${activeManager === name ? 'active' : ''}`}
                  onClick={() => setActiveManager(name)}
                  disabled={!installed}
                  title={!installed ? `${label} is not installed` : manager?.error}
                >
                  <span className={`manager-dot manager-${name}`} />
                  <span>{label}</span>
                  <span className="filter-count">{installed ? packageCount : '—'}</span>
                </button>
              )
            })}
          </div>
        </section>

        {managers.some((manager) => manager.installed && manager.error) && (
          <div className="manager-errors">
            {managers.filter((manager) => manager.installed && manager.error).map((manager) => (
              <p key={manager.name}><strong>{manager.name}:</strong> {manager.error}</p>
            ))}
          </div>
        )}
        {notice && (
          <div className={`notice notice-${notice.kind}`} role="status">
            <Icon name={notice.kind === 'success' ? 'check' : 'alert'} />
            <div className="notice-content">
              <span>{notice.message}</span>
              {notice.output && <details open={notice.kind === 'error'}>
                <summary>Command output</summary>
                <pre>{notice.output}</pre>
              </details>}
            </div>
            <button aria-label="Dismiss message" onClick={() => setNotice(null)}>×</button>
          </div>
        )}

        <section className="packages-panel" aria-label="Global packages">
          <div className="packages-toolbar">
            <div>
              <h2>{activeManager === 'all' ? 'Packages' : `${activeManager} packages`}</h2>
              <span>{loading ? 'Scanning' : `${filteredPackages.length} shown`}</span>
            </div>
            <div className="toolbar-actions">
              <label className="search-box">
                <Icon name="search" />
                <input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search packages" aria-label="Search packages" />
              </label>
              <button className="refresh-button" onClick={() => void loadManagers()} disabled={refreshing || appUpdating}>
                <span className={refreshing ? 'spin' : ''}><Icon name="refresh" /></span>
                <span>{refreshing ? 'Refreshing' : 'Refresh'}</span>
              </button>
            </div>
          </div>

          <div className="package-table">
            <div className="table-heading" aria-hidden="true">
              <span>Package</span><span>Manager</span><span>Version</span><span className="actions-heading">Actions</span>
            </div>
            {loading && <div className="empty-state" role="status"><span className="loading-spinner" aria-hidden="true" />Scanning</div>}
            {!loading && filteredPackages.map((pkg) => {
              const key = `${pkg.manager}:${pkg.name}`
              const pendingAction = activeActions[key]
              const latestVersion = latestVersions[key]
              const isLatest = Boolean(pkg.version && latestVersion && normalizeVersion(pkg.version) === normalizeVersion(latestVersion))
              const label = pkg.manager === 'bun' ? 'Bun' : pkg.manager
              const selfUpdate = usesPnpmSelfUpdate(pkg.manager, pkg.name)
              const actionsDisabled = appUpdating || Boolean(pendingAction) || (pkg.manager === 'pnpm' && pnpmSelfUpdating)
              const updateDisabled = actionsDisabled || (selfUpdate && pnpmBusy)
              return (
                <div className="package-row" key={key} aria-busy={Boolean(pendingAction)}>
                  <span className="package-name">
                    <span>{pkg.name}</span>
                    {selfUpdate && <small title="Updates the pnpm package manager using pnpm self-update">pnpm package manager · Uses pnpm self-update</small>}
                  </span>
                  <span className="package-manager"><i className={`manager-dot manager-${pkg.manager}`} />{label}</span>
                  <span className={`package-version ${!pkg.version ? 'package-version-missing' : ''}`} title={!pkg.version ? selfUpdate ? 'The installed version could not be read. Self-update updates the pnpm package manager.' : 'The installed version could not be read. Repair reinstalls the latest version.' : undefined}>{pkg.version || 'Incomplete install'}</span>
                  <div className="row-actions">
                    {pendingAction ? (
                      <span className="action-progress" role="status" aria-label={`${pkg.name}: ${pendingAction.stage}`}>
                        <span className="loading-spinner action-spinner" aria-hidden="true" />{pendingAction.stage}
                      </span>
                    ) : !pkg.version ? (
                      <button className="update-button" onClick={() => void runAction(pkg.manager, pkg, 'update')} disabled={updateDisabled}>{selfUpdate ? 'Self-update' : 'Repair'}</button>
                    ) : latestVersion === undefined ? (
                      <button className="update-button" disabled>Checking</button>
                    ) : latestVersion === null ? (
                      <button className="update-button" title="Could not check the latest version. Retry the version check." onClick={() => void checkLatestVersions(managers)} disabled={appUpdating}>Retry</button>
                    ) : isLatest ? (
                      <span className="latest-status" title={`Latest version ${latestVersion}`}><Icon name="check" />Latest</span>
                    ) : (
                      <button className="update-button" onClick={() => void runAction(pkg.manager, pkg, 'update')} disabled={updateDisabled}>{selfUpdate ? 'Self-update' : 'Update'}</button>
                    )}
                    {!pendingAction && <button className="delete-button" onClick={() => void runAction(pkg.manager, pkg, 'remove')} disabled={actionsDisabled} aria-label={`Delete ${pkg.name}`} title="Delete package">
                      <Icon name="trash" />
                    </button>}
                  </div>
                </div>
              )
            })}
            {!loading && filteredPackages.length === 0 && (
              <div className="empty-state">
                <strong>{search ? 'No packages match your search' : installedManagers.length ? 'No global packages found' : 'No package managers found'}</strong>
                <span>{search ? 'Try a different package name.' : 'Installed global packages will appear here.'}</span>
              </div>
            )}
          </div>
        </section>
        <AppUpdates packagesBusy={refreshing || Object.keys(activeActions).length > 0} onUpdating={setAppUpdating} />
      </div>
    </main>
  )
}

export default App
