import { useCallback, useEffect, useRef, useState } from 'react'
import { Events } from '@wailsio/runtime'
import * as UpdateService from '../bindings/depmanager/updates/service'

type UpdateInfo = {
  currentVersion: string
  latestVersion?: string
  available: boolean
  canInstall: boolean
  message: string
  releaseURL: string
  lastError?: string
  developmentBuild: boolean
}

function readPreference(key: string, fallback: string) {
  try { return localStorage.getItem(key) ?? fallback } catch { return fallback }
}

function savePreference(key: string, value: string) {
  try { localStorage.setItem(key, value) } catch { /* Updating still works when storage is unavailable. */ }
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : String(error)
}

export default function AppUpdates({ packagesBusy, onUpdating }: { packagesBusy: boolean, onUpdating: (busy: boolean) => void }) {
  const [info, setInfo] = useState<UpdateInfo | null>(null)
  const [checking, setChecking] = useState(false)
  const [installing, setInstalling] = useState(false)
  const [stage, setStage] = useState('Checking')
  const [message, setMessage] = useState('Checking for application updates…')
  const [error, setError] = useState<string | null>(null)
  const [automatic, setAutomatic] = useState(() => readPreference('depmanager.autoUpdates', 'true') === 'true')
  const busy = useRef(false)
  const lastAutomaticAttempt = useRef(readPreference('depmanager.lastAutoUpdate', ''))

  const check = useCallback(async () => {
    if (busy.current) return
    busy.current = true
    setChecking(true)
    setStage('Checking')
    setError(null)
    setMessage('Checking GitHub releases…')
    try {
      const result = await UpdateService.CheckForUpdates()
      setInfo(result)
      setMessage(result.message)
      if (result.lastError) setError(`The previous update failed: ${result.lastError}`)
    } catch (error) {
      setError(errorMessage(error))
      setMessage('Update check failed. You can try again.')
    } finally {
      busy.current = false
      setChecking(false)
      setStage('')
    }
  }, [])

  const install = useCallback(async () => {
    if (busy.current || packagesBusy) return
    busy.current = true
    setInstalling(true)
    setStage('Downloading')
    onUpdating(true)
    setError(null)
    setMessage('Downloading update…')
    try {
      await UpdateService.InstallUpdate()
      setStage('Restarting')
      setMessage('Restarting DepManager to finish the update…')
    } catch (error) {
      setError(errorMessage(error))
      setMessage('Update failed. You can retry with Update now.')
      busy.current = false
      setInstalling(false)
      setStage('')
      onUpdating(false)
    }
  }, [onUpdating, packagesBusy])

  useEffect(() => {
    let disposed = false
    void UpdateService.GetAppInfo().then((result) => {
      if (!disposed) setInfo((current) => current ?? result)
    }).catch(() => { /* The check below reports connection failures. */ })
    // Defer the initial request so StrictMode cleanup cancels its first pass.
    const initialCheck = window.setTimeout(() => void check(), 0)
    const interval = window.setInterval(() => void check(), 6 * 60 * 60 * 1000)
    const removeProgress = Events.On('wails:updater:download-progress', (event) => {
      setStage('Downloading')
      const progress = event.data as { written: number, total: number }
      if (progress.total > 0) setMessage(`Downloading update… ${Math.min(100, Math.round(progress.written / progress.total * 100))}%`)
    })
    const removeVerifying = Events.On('wails:updater:verifying', () => {
      setStage('Verifying')
      setMessage('Verifying update…')
    })
    const removeReady = Events.On('wails:updater:update-ready', () => {
      setStage('Installing')
      setMessage('Installing update… DepManager will restart when finished.')
    })
    return () => {
      disposed = true
      window.clearTimeout(initialCheck)
      window.clearInterval(interval)
      removeProgress()
      removeVerifying()
      removeReady()
    }
  }, [check])

  useEffect(() => {
    if (!automatic || !info?.available || !info.canInstall || checking || installing || packagesBusy || error) return
    if (lastAutomaticAttempt.current === info.latestVersion) return
    // Remember failed attempts across restarts to prevent an update/restart loop.
    lastAutomaticAttempt.current = info.latestVersion ?? ''
    savePreference('depmanager.lastAutoUpdate', lastAutomaticAttempt.current)
    void install()
  }, [automatic, checking, error, info, install, installing, packagesBusy])

  return (
    <section className="app-updates" aria-label="Application updates" aria-busy={checking || installing}>
      <div className="app-update-details">
        <strong>DepManager{info ? ` v${info.currentVersion}` : ''}</strong>
        <span role="status" title={message}>{stage || message}</span>
        {info && !info.canInstall && <span>{info.developmentBuild ? 'Development build: automatic installation is disabled.' : 'Automatic installation is unavailable for this platform.'}</span>}
        {error && <span className="app-update-error" role="alert">{error}</span>}
      </div>
      <div className="app-update-controls">
        <label className="auto-update-option">
          <input type="checkbox" checked={automatic} disabled={installing || !info?.canInstall} onChange={(event) => {
            setAutomatic(event.target.checked)
            savePreference('depmanager.autoUpdates', String(event.target.checked))
          }} />
          Automatically install updates
        </label>
        <div className="app-update-buttons">
          <button className="refresh-button" disabled={checking || installing} onClick={() => void check()}>
            {(checking || installing) && <span className="loading-spinner app-update-spinner" aria-hidden="true" />}
            {checking || installing ? stage : 'Check for updates'}
          </button>
          {info?.available && info.canInstall && <button className="update-button" disabled={checking || installing || packagesBusy} onClick={() => void install()} title={packagesBusy ? 'Wait for package operations to finish' : undefined}>Update now</button>}
          {info?.available && !info.canInstall && <a href={info.releaseURL} target="_blank" rel="noreferrer">View release</a>}
        </div>
      </div>
    </section>
  )
}
