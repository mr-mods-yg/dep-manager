import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import * as PackageService from '../bindings/depdesk/packageservice'

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
}

const managerNames = ['npm', 'pnpm', 'bun']

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
  const [search, setSearch] = useState('')
  const [activeManager, setActiveManager] = useState('all')
  const [activeActions, setActiveActions] = useState<Record<string, 'update' | 'remove'>>({})
  const [latestVersions, setLatestVersions] = useState<Record<string, string | null | undefined>>({})
  const [notice, setNotice] = useState<Notice | null>(null)
  const latestCheckRun = useRef(0)

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
      while (nextPackage < packages.length) {
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

    await Promise.all(Array.from({ length: Math.min(4, packages.length) }, () => checkWorker()))
  }, [])

  const loadManagers = useCallback(async (showLoading = false) => {
    if (showLoading) setLoading(true)
    setRefreshing(true)
    try {
      const data = await PackageService.GetPackageManagers()
      const normalized = (data ?? []).map((manager) => ({ ...manager, packages: manager.packages ?? [] }))
      setManagers(normalized)
      void checkLatestVersions(normalized)
    } catch (error) {
      setNotice({ kind: 'error', message: error instanceof Error ? error.message : 'Could not read package managers.' })
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [checkLatestVersions])

  useEffect(() => { void loadManagers(true) }, [loadManagers])

  const installedManagers = useMemo(() => managers.filter((manager) => manager.installed), [managers])
  const allPackages = useMemo(() => installedManagers.flatMap((manager) => manager.packages.map((pkg) => ({ ...pkg, manager: manager.name }))), [installedManagers])
  const filteredPackages = useMemo(() => {
    const query = search.trim().toLowerCase()
    return allPackages.filter((pkg) => (activeManager === 'all' || pkg.manager === activeManager) && (!query || `${pkg.name} ${pkg.version} ${pkg.manager}`.toLowerCase().includes(query)))
  }, [activeManager, allPackages, search])
  const runAction = async (manager: string, pkg: GlobalPackage, action: 'update' | 'remove') => {
    if (action === 'remove' && !window.confirm(`Remove ${pkg.name} from ${manager}'s global packages?`)) return
    const key = `${manager}:${pkg.name}`
    setActiveActions((current) => ({ ...current, [key]: action }))
    setNotice(null)
    try {
      const result = await PackageService.RunPackageAction(manager, pkg.name, action)
      if (!result.success) {
        setNotice({ kind: 'error', message: result.message })
      } else {
        setNotice({ kind: 'success', message: result.message })
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
            <span>{notice.message}</span>
            <button aria-label="Dismiss message" onClick={() => setNotice(null)}>×</button>
          </div>
        )}

        <section className="packages-panel" aria-label="Global packages">
          <div className="packages-toolbar">
            <div>
              <h2>{activeManager === 'all' ? 'Packages' : `${activeManager} packages`}</h2>
              <span>{loading ? 'Scanning…' : `${filteredPackages.length} shown`}</span>
            </div>
            <div className="toolbar-actions">
              <label className="search-box">
                <Icon name="search" />
                <input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search packages" aria-label="Search packages" />
              </label>
              <button className="refresh-button" onClick={() => void loadManagers()} disabled={refreshing}>
                <span className={refreshing ? 'spin' : ''}><Icon name="refresh" /></span>
                <span>{refreshing ? 'Refreshing' : 'Refresh'}</span>
              </button>
            </div>
          </div>

          <div className="package-table">
            <div className="table-heading" aria-hidden="true">
              <span>Package</span><span>Manager</span><span>Version</span><span className="actions-heading">Actions</span>
            </div>
            {loading && <div className="empty-state"><span className="loading-spinner" />Scanning installed packages…</div>}
            {!loading && filteredPackages.map((pkg) => {
              const key = `${pkg.manager}:${pkg.name}`
              const pendingAction = activeActions[key]
              const latestVersion = latestVersions[key]
              const isLatest = Boolean(pkg.version && latestVersion && normalizeVersion(pkg.version) === normalizeVersion(latestVersion))
              const label = pkg.manager === 'bun' ? 'Bun' : pkg.manager
              return (
                <div className="package-row" key={key} aria-busy={Boolean(pendingAction)}>
                  <span className="package-name">{pkg.name}</span>
                  <span className="package-manager"><i className={`manager-dot manager-${pkg.manager}`} />{label}</span>
                  <span className="package-version">{pkg.version || '—'}</span>
                  <div className="row-actions">
                    {pendingAction === 'update' ? (
                      <button className="update-button" disabled>Updating…</button>
                    ) : latestVersion === undefined ? (
                      <button className="update-button" disabled>Checking…</button>
                    ) : isLatest ? (
                      <span className="latest-status" title={`Latest version ${latestVersion}`}><Icon name="check" />Latest</span>
                    ) : (
                      <button className="update-button" onClick={() => void runAction(pkg.manager, pkg, 'update')} disabled={Boolean(pendingAction)}>Update</button>
                    )}
                    <button className={`delete-button ${pendingAction === 'remove' ? 'is-removing' : ''}`} onClick={() => void runAction(pkg.manager, pkg, 'remove')} disabled={Boolean(pendingAction)} aria-label={pendingAction === 'remove' ? `Removing ${pkg.name}` : `Delete ${pkg.name}`} title={pendingAction === 'remove' ? 'Removing package' : 'Delete package'}>
                      {pendingAction === 'remove' ? 'Removing…' : <Icon name="trash" />}
                    </button>
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
      </div>
    </main>
  )
}

export default App
