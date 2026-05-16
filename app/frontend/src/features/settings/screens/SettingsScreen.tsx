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
import {
  User,
  Mail,
  Phone,
  ShieldCheck,
  Save,
  Download,
  Camera,
  Trash2,
  X,
  Lock,
  Info,
  BadgeCheck,
  Settings2,
} from 'lucide-react'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '../../../components/ui/card'
import { Button } from '../../../components/ui/button'
import { Input } from '../../../components/ui/input'
import { Label } from '../../../components/ui/label'
import { Separator } from '../../../components/ui/separator'
import { cn } from '@/lib/utils'

export default function SettingsScreen() {
  const { pushToast } = useToastStore()
  const [activeTab, setActiveTab] = useState<'profile' | 'security'>('profile')
  const [passphrase, setPassphrase] = useState('')

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
    if (!passphrase) return
    try {
      await runtimeClient.exportIdentity(passphrase)
      pushToast({
        title: 'Đã xuất bản sao lưu',
        description: 'Vui lòng cất giữ tệp .backup và mật khẩu an toàn.',
        variant: 'default',
      })
      setPassphrase('')
    } catch (err) {
      pushToast({
        title: 'Lỗi sao lưu',
        description: String(err),
        variant: 'destructive',
      })
    }
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
      try {
        const out = await compressAvatarFile(f)
        if (out.outputBytes > AVATAR_OUTPUT_MAX_BYTES) {
          throw new AvatarImageError(`Ảnh sau xử lý vẫn vượt ${formatBytesShort(AVATAR_OUTPUT_MAX_BYTES)}.`)
        }
        setRemoveAvatarOnSave(false)
        revokeAvatarPreview()
        setAvatarPreviewUrl(URL.createObjectURL(out.blob))
        setPendingCompressedAvatar(out)
        
        pushToast({
          title: 'Đã nạp ảnh mới',
          description: out.wasCompressed ? `Ảnh đã được tối ưu (${formatBytesShort(out.outputBytes)}).` : 'Ảnh đã sẵn sàng để lưu.',
          variant: 'default',
        })
      } catch (err) {
        const msg = err instanceof AvatarImageError ? err.message : err instanceof Error ? err.message : String(err)
        pushToast({ title: 'Lỗi xử lý ảnh', description: msg, variant: 'destructive' })
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
          pushToast({
            title: 'Ảnh quá lớn',
            description: `Dung lượng phải ≤ ${formatBytesShort(AVATAR_OUTPUT_MAX_BYTES)}.`,
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
      
      pushToast({
        title: 'Cập nhật thành công',
        description: 'Hồ sơ của bạn đã được lưu và đồng bộ P2P.',
        variant: 'default',
      })
    } catch (err) {
      const raw = err instanceof Error ? err.message : String(err)
      pushToast({
        title: 'Lỗi lưu hồ sơ',
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
    <div className="flex h-full flex-col bg-slate-950/10 p-6 md:p-10 space-y-8 overflow-y-auto custom-scrollbar">
      {/* Header Section */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between border-b border-slate-800/60 pb-8">
        <div className="space-y-1">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-emerald-500/10 ring-1 ring-emerald-500/20">
              <Settings2 className="h-4.5 w-4.5 text-emerald-500" />
            </div>
            <h1 className="text-2xl font-bold tracking-tight text-slate-100">Cài đặt</h1>
          </div>
          <p className="text-slate-400 text-xs">Quản lý định danh và cấu hình cá nhân của bạn.</p>
        </div>

        {/* Tab Switcher */}
        <div className="flex p-1 rounded-xl bg-slate-950/60 border border-slate-800/60 w-fit">
          <button
            onClick={() => setActiveTab('profile')}
            className={cn(
              "flex items-center gap-2 px-6 py-2 text-xs font-semibold rounded-lg transition-all",
              activeTab === 'profile' 
                ? "bg-emerald-600 text-white shadow-lg shadow-emerald-900/20" 
                : "text-slate-400 hover:bg-slate-800 hover:text-slate-200"
            )}
          >
            <User className="h-3.5 w-3.5" />
            Hồ sơ
          </button>
          <button
            onClick={() => setActiveTab('security')}
            className={cn(
              "flex items-center gap-2 px-6 py-2 text-xs font-semibold rounded-lg transition-all",
              activeTab === 'security' 
                ? "bg-emerald-600 text-white shadow-lg shadow-emerald-900/20" 
                : "text-slate-400 hover:bg-slate-800 hover:text-slate-200"
            )}
          >
            <ShieldCheck className="h-3.5 w-3.5" />
            Bảo mật
          </button>
        </div>
      </div>

      <input
        ref={fileInputRef}
        type="file"
        accept="image/png,image/jpeg,image/webp,.png,.jpg,.jpeg,.webp"
        className="hidden"
        onChange={handleAvatarFileChange}
      />

      <div className="flex justify-center h-full">
        <div className="w-full max-w-4xl animate-in fade-in slide-in-from-bottom-2 duration-300">
          {activeTab === 'profile' ? (
            <Card className="border-slate-800 bg-slate-900/40 shadow-xl backdrop-blur-sm overflow-hidden">
              <CardHeader className="pb-8 border-b border-slate-800/40 bg-slate-900/20">
                <div className="flex items-center gap-2.5 text-emerald-500">
                  <User className="h-5 w-5" />
                  <CardTitle className="text-xl">Hồ sơ cá nhân</CardTitle>
                </div>
                <CardDescription className="text-slate-400 text-xs mt-1">
                  Thông tin này giúp đồng nghiệp nhận diện bạn trong các cuộc hội thoại.
                </CardDescription>
              </CardHeader>
              <CardContent className="pt-10 space-y-10">
                {/* Avatar Area */}
                <div className="flex flex-col sm:flex-row items-center gap-10">
                  <div className="relative group">
                    <div className="h-28 w-28 rounded-full border-2 border-slate-700 bg-slate-800 overflow-hidden ring-4 ring-slate-900/50 group-hover:ring-emerald-500/20 transition-all shadow-2xl">
                      {displayAvatarSrc ? (
                        <img
                          src={displayAvatarSrc}
                          alt="Avatar"
                          className="h-full w-full object-cover"
                        />
                      ) : (
                        <div className="flex h-full w-full items-center justify-center bg-slate-800 text-slate-500">
                          <User className="h-12 w-12" />
                        </div>
                      )}
                      {avatarProcessing && (
                        <div className="absolute inset-0 flex items-center justify-center bg-slate-900/60 backdrop-blur-[1px]">
                          <div className="h-8 w-8 animate-spin rounded-full border-2 border-emerald-500/20 border-t-emerald-500" />
                        </div>
                      )}
                    </div>
                    <Button
                      size="icon"
                      variant="secondary"
                      className="absolute -bottom-1 -right-1 h-9 w-9 rounded-full shadow-xl border border-slate-700 bg-slate-800 hover:bg-slate-700 text-slate-200"
                      onClick={handlePickAvatarClick}
                      disabled={profileSaving || avatarProcessing}
                    >
                      <Camera className="h-4.5 w-4.5" />
                    </Button>
                  </div>
                  <div className="flex-1 space-y-3 text-center sm:text-left">
                    <h4 className="text-sm font-bold text-slate-200 uppercase tracking-tight">Ảnh đại diện</h4>
                    <p className="text-[11px] text-slate-500 leading-relaxed max-w-sm">
                      Định dạng PNG, JPEG hoặc WebP. Hệ thống tự động tối ưu dung lượng xuống dưới {AVATAR_OUTPUT_MAX_BYTES / 1024} KiB để tiết kiệm băng thông P2P.
                    </p>
                    <div className="flex items-center justify-center sm:justify-start gap-3 pt-1">
                      {pendingCompressedAvatar && (
                        <Button variant="ghost" size="sm" className="h-8 text-[10px] text-rose-400 hover:text-rose-300 hover:bg-rose-500/10 font-semibold" onClick={handleDiscardAvatarDraft}>
                          <X className="mr-1.5 h-3.5 w-3.5" /> Hủy thay đổi
                        </Button>
                      )}
                      {profile?.avatar_hash && !pendingCompressedAvatar && !removeAvatarOnSave && (
                        <Button variant="ghost" size="sm" className="h-8 text-[10px] text-slate-500 hover:text-rose-400 hover:bg-rose-500/10 font-semibold" onClick={handleMarkRemoveSavedAvatar}>
                          <Trash2 className="mr-1.5 h-3.5 w-3.5" /> Gỡ bỏ ảnh hiện tại
                        </Button>
                      )}
                    </div>
                  </div>
                </div>

                <Separator className="bg-slate-800/60" />

                {/* Input Fields */}
                <div className="grid gap-8">
                  <div className="space-y-3">
                    <div className="flex items-center justify-between">
                      <Label className="text-[11px] font-bold uppercase tracking-wider text-slate-500 ml-1">Tên hiển thị (MLS Identity)</Label>
                      <div className="flex items-center gap-1 rounded bg-blue-500/10 px-2 py-0.5 text-[9px] font-bold text-blue-400 ring-1 ring-inset ring-blue-500/20 shadow-sm shadow-blue-900/10">
                        <BadgeCheck className="h-2.5 w-2.5" />
                        XÁC THỰC BỞI ADMIN
                      </div>
                    </div>
                    <div className="relative group">
                      <User className="absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-600" />
                      <Input
                        readOnly
                        value={profile?.display_name ?? ''}
                        className="bg-slate-950/40 border-slate-800/60 pl-10 text-xs h-11 text-slate-400 cursor-not-allowed border-dashed"
                      />
                    </div>
                    <p className="text-[10px] text-slate-600 italic px-1">Đây là định danh gốc được ký bởi Quản trị viên, không thể thay đổi.</p>
                  </div>

                  <div className="grid gap-8 sm:grid-cols-2">
                    <div className="space-y-3">
                      <Label className="text-[11px] font-bold uppercase tracking-wider text-slate-500 ml-1">Email liên hệ</Label>
                      <div className="relative group">
                        <Mail className="absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500 transition-colors group-focus-within:text-emerald-500" />
                        <Input
                          value={emailDraft}
                          onChange={(e) => setEmailDraft(e.target.value)}
                          placeholder="ten@congty.com"
                          className="bg-slate-950/40 border-slate-700/60 pl-10 text-xs h-11 focus:ring-emerald-500/10 transition-all"
                        />
                      </div>
                    </div>
                    <div className="space-y-3">
                      <Label className="text-[11px] font-bold uppercase tracking-wider text-slate-500 ml-1">Số điện thoại</Label>
                      <div className="relative group">
                        <Phone className="absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500 transition-colors group-focus-within:text-emerald-500" />
                        <Input
                          value={phoneDraft}
                          onChange={(e) => setPhoneDraft(e.target.value)}
                          placeholder="09xx xxx xxx"
                          className="bg-slate-950/40 border-slate-700/60 pl-10 text-xs h-11 focus:ring-emerald-500/10 transition-all"
                        />
                      </div>
                    </div>
                  </div>
                </div>
              </CardContent>
              <CardFooter className="bg-slate-900/20 border-t border-slate-800/40 p-8 flex justify-end">
                <Button
                  className="bg-emerald-600 hover:bg-emerald-500 text-white font-bold px-10 h-11 transition-all shadow-lg shadow-emerald-900/20 hover:scale-[1.02] active:scale-[0.98]"
                  onClick={() => void handleSaveProfile()}
                  disabled={profileSaving || avatarProcessing}
                >
                  {profileSaving ? (
                    <div className="flex items-center gap-2">
                      <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/20 border-t-white" />
                      Đang lưu hồ sơ...
                    </div>
                  ) : (
                    <>
                      <Save className="mr-2 h-4.5 w-4.5" />
                      Cập nhật Hồ sơ
                    </>
                  )}
                </Button>
              </CardFooter>
            </Card>
          ) : (
            <Card className="border-slate-800 bg-slate-900/40 shadow-xl backdrop-blur-sm overflow-hidden h-full">
              <CardHeader className="pb-8 border-b border-slate-800/40 bg-slate-900/20">
                <div className="flex items-center gap-2.5 text-blue-400">
                  <ShieldCheck className="h-5 w-5" />
                  <CardTitle className="text-xl">Bảo mật & Khôi phục</CardTitle>
                </div>
                <CardDescription className="text-slate-400 text-xs mt-1">
                  Quản lý mã hóa và các phương thức khôi phục danh tính.
                </CardDescription>
              </CardHeader>
              <CardContent className="pt-10 space-y-8">
                <div className="rounded-2xl bg-blue-500/5 border border-blue-500/10 p-6 space-y-4 shadow-inner shadow-blue-900/5">
                  <div className="flex gap-4">
                    <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-blue-500/10">
                      <Info className="h-5 w-5 text-blue-400" />
                    </div>
                    <div className="space-y-2">
                      <p className="text-xs font-bold text-blue-100 uppercase tracking-widest">Tầm quan trọng của sao lưu</p>
                      <p className="text-[11px] leading-relaxed text-slate-400 max-w-2xl">
                        Vì hệ thống hoạt động phi tập trung (P2P), định danh và dữ liệu của bạn chỉ tồn tại trên thiết bị này. 
                        Tệp sao lưu chứa <span className="text-blue-300 font-semibold">Khóa bí mật (Private Key)</span> đã được mã hóa. 
                        Bạn <span className="text-rose-400 font-bold underline">phải cất giữ</span> tệp này cùng mật khẩu để có thể đăng nhập trên thiết bị khác.
                      </p>
                    </div>
                  </div>
                </div>

                <div className="space-y-4 max-w-lg">
                  <Label htmlFor="backup-pass" className="text-[11px] font-bold uppercase tracking-wider text-slate-500 ml-1">Mật khẩu bảo vệ tệp sao lưu</Label>
                  <div className="relative group">
                    <Lock className="absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500 transition-colors group-focus-within:text-blue-500" />
                    <Input
                      id="backup-pass"
                      type="password"
                      value={passphrase}
                      onChange={(e) => setPassphrase(e.target.value)}
                      placeholder="Nhập mật khẩu mới để mã hóa tệp..."
                      className="bg-slate-950/40 border-slate-700/60 pl-10 text-xs h-11 focus:ring-blue-500/10 transition-all font-mono"
                    />
                  </div>
                  <div className="flex items-start gap-2 px-1">
                    <ShieldCheck className="h-3 w-3 text-emerald-500 shrink-0 mt-0.5" />
                    <p className="text-[10px] text-slate-500 leading-tight">
                      Mật khẩu này không được lưu ở bất kỳ đâu. Hãy ghi nhớ hoặc sử dụng trình quản lý mật khẩu.
                    </p>
                  </div>
                </div>
              </CardContent>
              <CardFooter className="bg-slate-900/20 border-t border-slate-800/40 p-8">
                <Button
                  variant="outline"
                  className="w-full sm:w-fit px-10 h-11 border-blue-500/30 bg-blue-500/5 text-blue-400 hover:bg-blue-500/10 hover:border-blue-500/50 font-bold transition-all shadow-lg shadow-blue-900/5 hover:scale-[1.01]"
                  onClick={() => void handleExport()}
                  disabled={!passphrase.trim()}
                >
                  <Download className="mr-2.5 h-4.5 w-4.5" />
                  Xuất bản sao lưu định danh (.backup)
                </Button>
              </CardFooter>
            </Card>
          )}
        </div>
      </div>
    </div>
  )
}
