import { useCallback, useEffect, useRef, useState, type ChangeEvent } from 'react'
import { service } from '../../../../wailsjs/go/models'
import { runtimeClient } from '../../../services/runtime/runtimeClient'
import { useToastStore } from '../../../stores/useToastStore'
import {
  AVATAR_INPUT_MAX_BYTES,
  AVATAR_OUTPUT_MAX_BYTES,
  AvatarImageError,
  compressAvatarFile,
  formatBytesShort,
  type CompressedAvatarResult,
} from '../../../lib/avatarImage'

export default function SettingsScreen() {
  const pushToast = useToastStore((s) => s.pushToast)
  const [passphrase, setPassphrase] = useState('')
  const [bootstrap, setBootstrap] = useState('')
  const [status, setStatus] = useState('')
  const [statusTone, setStatusTone] = useState<'success' | 'error'>('success')

  const [profileLoading, setProfileLoading] = useState(false)
  const [profileSaving, setProfileSaving] = useState(false)
  const [profile, setProfile] = useState<service.UserProfileInfo | null>(null)
  const [emailDraft, setEmailDraft] = useState('')
  const [phoneDraft, setPhoneDraft] = useState('')

  const fileInputRef = useRef<HTMLInputElement>(null)
  const [pendingCompressedAvatar, setPendingCompressedAvatar] = useState<CompressedAvatarResult | null>(null)
  const [avatarPreviewUrl, setAvatarPreviewUrl] = useState<string | null>(null)
  const [removeAvatarOnSave, setRemoveAvatarOnSave] = useState(false)
  const [avatarProcessing, setAvatarProcessing] = useState(false)

  const revokeAvatarPreview = useCallback(() => {
    setAvatarPreviewUrl((prev) => {
      if (prev) URL.revokeObjectURL(prev)
      return null
    })
  }, [])

  const loadProfile = useCallback(async () => {
    setProfileLoading(true)
    try {
      const p = await runtimeClient.getMyProfile()
      setProfile(p)
      setEmailDraft(p.email ?? '')
      setPhoneDraft(p.phone ?? '')
      setPendingCompressedAvatar(null)
      setRemoveAvatarOnSave(false)
      revokeAvatarPreview()
    } catch (err) {
      const raw = err instanceof Error ? err.message : String(err)
      setStatusTone('error')
      setStatus(`Không tải được hồ sơ: ${raw}`)
      pushToast({ title: 'Không tải được hồ sơ', description: raw, variant: 'destructive' })
    } finally {
      setProfileLoading(false)
    }
  }, [revokeAvatarPreview, pushToast])

  useEffect(() => {
    void loadProfile()
  }, [loadProfile])

  useEffect(() => {
    return () => {
      revokeAvatarPreview()
    }
  }, [revokeAvatarPreview])

  const handleExport = async () => {
    await runtimeClient.exportIdentity(passphrase)
    setStatusTone('success')
    setStatus('Identity backup exported.')
  }

  const handleReconnect = async () => {
    if (bootstrap.trim()) {
      await runtimeClient.validateMultiaddr(bootstrap.trim())
      await runtimeClient.setBootstrapAddress(bootstrap.trim())
    }
    await runtimeClient.reconnectP2P()
    setStatusTone('success')
    setStatus('P2P reconnected.')
  }

  const handleExportDiagnostics = async () => {
    const path = await runtimeClient.exportDiagnostics()
    setStatusTone('success')
    setStatus(`Diagnostics exported: ${path}`)
  }

  const handlePickAvatarClick = () => {
    fileInputRef.current?.click()
  }

  const handleAvatarFileChange = (e: ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0]
    e.target.value = ''
    if (!f) return

    void (async () => {
      setAvatarProcessing(true)
      setStatus('')
      try {
        const out = await compressAvatarFile(f)
        if (out.outputBytes > AVATAR_OUTPUT_MAX_BYTES) {
          throw new AvatarImageError(`Ảnh sau xử lý vẫn vượt ${formatBytesShort(AVATAR_OUTPUT_MAX_BYTES)}.`)
        }
        setRemoveAvatarOnSave(false)
        revokeAvatarPreview()
        setAvatarPreviewUrl(URL.createObjectURL(out.blob))
        setPendingCompressedAvatar(out)
        if (out.wasCompressed) {
          pushToast({
            title: 'Đã tối ưu ảnh',
            description: `Từ ${formatBytesShort(out.originalBytes)} → ${formatBytesShort(out.outputBytes)} (${out.width}×${out.height}). Bấm «Lưu hồ sơ» để gửi.`,
            variant: 'default',
          })
        }
      } catch (err) {
        const msg = err instanceof AvatarImageError ? err.message : err instanceof Error ? err.message : String(err)
        setStatusTone('error')
        setStatus(msg)
        pushToast({ title: 'Không xử lý được ảnh', description: msg, variant: 'destructive' })
        setPendingCompressedAvatar(null)
        revokeAvatarPreview()
      } finally {
        setAvatarProcessing(false)
      }
    })()
  }

  const handleDiscardAvatarDraft = () => {
    setPendingCompressedAvatar(null)
    revokeAvatarPreview()
  }

  const handleMarkRemoveSavedAvatar = () => {
    setPendingCompressedAvatar(null)
    revokeAvatarPreview()
    setRemoveAvatarOnSave(true)
  }

  const handleSaveProfile = async () => {
    setProfileSaving(true)
    const pendingSnap = pendingCompressedAvatar
    try {
      let avatarChange = 0
      let avatarBytes: number[] = []
      if (removeAvatarOnSave) {
        avatarChange = 2
      } else if (pendingSnap) {
        if (pendingSnap.outputBytes > AVATAR_OUTPUT_MAX_BYTES) {
          setStatusTone('error')
          setStatus(`Ảnh sau nén vượt ${formatBytesShort(AVATAR_OUTPUT_MAX_BYTES)}. Chọn ảnh khác.`)
          pushToast({
            title: 'Ảnh quá lớn',
            description: `Sau nén vẫn phải ≤ ${formatBytesShort(AVATAR_OUTPUT_MAX_BYTES)}.`,
            variant: 'destructive',
          })
          return
        }
        avatarBytes = pendingSnap.bytes
        avatarChange = 1
      }

      const next = await runtimeClient.saveMyProfile(
        new service.UpdateUserProfileRequest({
          email: emailDraft.trim(),
          phone: phoneDraft.trim(),
        }),
        avatarBytes,
        avatarChange,
      )
      setProfile(next)
      setPendingCompressedAvatar(null)
      setRemoveAvatarOnSave(false)
      revokeAvatarPreview()
      setStatusTone('success')
      setStatus('Đã lưu hồ sơ. Ảnh đại diện sẽ đồng bộ tới các peer đã kết nối (P2P).')
      const desc =
        avatarChange === 1 && pendingSnap?.wasCompressed
          ? `Đã lưu (ảnh đã nén còn ${formatBytesShort(pendingSnap.outputBytes)}).`
          : 'Thông tin và ảnh đại diện đã được cập nhật.'
      pushToast({
        title: 'Đã lưu hồ sơ',
        description: desc,
        variant: 'default',
      })
    } catch (err) {
      const raw = err instanceof Error ? err.message : String(err)
      setStatusTone('error')
      setStatus(`Lưu hồ sơ thất bại: ${raw}`)
      pushToast({
        title: 'Lưu hồ sơ thất bại',
        description: raw,
        variant: 'destructive',
      })
    } finally {
      setProfileSaving(false)
    }
  }

  const displayAvatarSrc =
    avatarPreviewUrl || (removeAvatarOnSave ? '' : (profile?.avatar_data_url ?? '')) || ''

  return (
    <div className="space-y-4 p-4 text-sm text-slate-200">
      <h3 className="font-semibold">Settings & Recovery</h3>

      <input
        ref={fileInputRef}
        type="file"
        accept="image/png,image/jpeg,image/webp,.png,.jpg,.jpeg,.webp"
        className="hidden"
        onChange={handleAvatarFileChange}
      />

      <div className="rounded-lg border border-slate-700 p-3">
        <p className="mb-2 text-xs text-slate-400">Hồ sơ (tên hiển thị theo Admin / MLS, không đổi tại đây)</p>
        {profileLoading && !profile ? (
          <p className="text-xs text-slate-500">Đang tải…</p>
        ) : (
          <div className="space-y-3">
            <div>
              <label className="mb-1 block text-[11px] font-medium text-slate-500">Tên hiển thị</label>
              <input
                readOnly
                value={profile?.display_name ?? ''}
                className="w-full cursor-not-allowed rounded border border-slate-700 bg-slate-900/80 px-2 py-1 text-xs text-slate-400"
              />
            </div>
            <div>
              <label className="mb-1 block text-[11px] font-medium text-slate-500">Email (tuỳ chọn)</label>
              <input
                value={emailDraft}
                onChange={(e) => setEmailDraft(e.target.value)}
                type="email"
                autoComplete="email"
                className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs"
              />
            </div>
            <div>
              <label className="mb-1 block text-[11px] font-medium text-slate-500">Điện thoại (tuỳ chọn)</label>
              <input
                value={phoneDraft}
                onChange={(e) => setPhoneDraft(e.target.value)}
                type="tel"
                autoComplete="tel"
                className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs"
              />
            </div>
            <div className="flex flex-wrap items-center gap-2">
              {displayAvatarSrc ? (
                <img
                  src={displayAvatarSrc}
                  alt=""
                  className="h-12 w-12 shrink-0 rounded-full border border-slate-600 object-cover"
                />
              ) : (
                <span className="text-xs text-slate-500">
                  Chưa có ảnh đại diện (PNG / JPEG / WebP). Chọn ảnh rồi bấm «Lưu hồ sơ».
                </span>
              )}
              <button
                type="button"
                className="btn-secondary text-xs"
                disabled={profileSaving || avatarProcessing}
                onClick={handlePickAvatarClick}
              >
                {avatarProcessing ? 'Đang xử lý ảnh…' : 'Chọn ảnh…'}
              </button>
              {pendingCompressedAvatar ? (
                <button type="button" className="btn-ghost text-xs" onClick={handleDiscardAvatarDraft}>
                  Bỏ ảnh đang chọn
                </button>
              ) : null}
              {profile?.avatar_hash && !pendingCompressedAvatar ? (
                <button type="button" className="btn-ghost text-xs" onClick={handleMarkRemoveSavedAvatar}>
                  Xóa ảnh khi lưu
                </button>
              ) : null}
            </div>
            <p className="text-[11px] leading-relaxed text-slate-500">
              Có thể chọn ảnh gốc tới {AVATAR_INPUT_MAX_BYTES / (1024 * 1024)} MiB; ứng dụng sẽ tự resize/nén còn tối đa{' '}
              {AVATAR_OUTPUT_MAX_BYTES / 1024} KiB trước khi lưu và đồng bộ P2P. Định dạng HEIC/GIF không được hỗ trợ.
            </p>
            <button
              type="button"
              className="btn-secondary text-xs"
              disabled={profileSaving || avatarProcessing}
              onClick={() => void handleSaveProfile()}
            >
              {profileSaving ? 'Đang lưu…' : 'Lưu hồ sơ'}
            </button>
          </div>
        )}
      </div>

      <div className="rounded-lg border border-slate-700 p-3">
        <p className="mb-2 text-xs text-slate-400">Identity backup export</p>
        <input
          value={passphrase}
          onChange={(event) => setPassphrase(event.target.value)}
          type="password"
          placeholder="Backup passphrase"
          className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs"
        />
        <button className="btn-secondary mt-2" onClick={() => void handleExport()} disabled={!passphrase}>
          Export backup
        </button>
      </div>
      <div className="rounded-lg border border-slate-700 p-3">
        <p className="mb-2 text-xs text-slate-400">Bootstrap runtime override</p>
        <input
          value={bootstrap}
          onChange={(event) => setBootstrap(event.target.value)}
          placeholder="/ip4/.../tcp/.../p2p/..."
          className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs"
        />
        <button className="btn-secondary mt-2" onClick={() => void handleReconnect()}>
          Save & reconnect
        </button>
      </div>
      <div className="rounded-lg border border-slate-700 p-3">
        <p className="mb-2 text-xs text-slate-400">Developer diagnostics</p>
        <div className="flex gap-2">
          <button className="btn-secondary text-xs" onClick={() => void handleExportDiagnostics()}>
            Export diagnostics
          </button>
          <button className="btn-ghost text-xs" onClick={() => void runtimeClient.openLogFolder()}>
            Open log folder
          </button>
        </div>
      </div>
      {status ? (
        <p className={`text-xs ${statusTone === 'error' ? 'text-red-400' : 'text-emerald-300'}`}>{status}</p>
      ) : null}
    </div>
  )
}
