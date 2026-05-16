import { ChangeEventHandler, useEffect, useState, useMemo } from 'react'
import { runtimeClient } from '../../../services/runtime/runtimeClient'
import { useWailsEvent } from '../../../hooks/useWailsEvent'
import { useToastStore } from '../../../stores/useToastStore'
import {
  ShieldCheck,
  Lock,
  Unlock,
  UserPlus,
  FileUp,
  History,
  Fingerprint,
  Key,
  User,
  AlertCircle,
  CheckCircle2,
  Clock,
  LogOut,
  ChevronRight,
  FileBadge,
  ShieldAlert,
} from 'lucide-react'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '../../../components/ui/card'
import { Button } from '../../../components/ui/button'
import { Input } from '../../../components/ui/input'
import { Label } from '../../../components/ui/label'
import { cn } from '@/lib/utils'

export default function AdminPanelScreen() {
  const { pushToast } = useToastStore()
  
  const [adminPasswordInput, setAdminPasswordInput] = useState('')
  const [adminPassphrase, setAdminPassphrase] = useState('')
  const [isAdminUnlocked, setIsAdminUnlocked] = useState(false)
  const [activeTab, setActiveTab] = useState<'manual' | 'file'>('manual')
  
  // Form State
  const [peerId, setPeerId] = useState('')
  const [mlsPublicKey, setMlsPublicKey] = useState('')
  const [displayName, setDisplayName] = useState('')
  
  const [adminReady, setAdminReady] = useState<boolean | null>(null)
  const [backendUnlocked, setBackendUnlocked] = useState(false)
  const [history, setHistory] = useState<Array<{ 
    id: string; 
    display_name: string; 
    peer_id: string; 
    issued_at: number;
    bundle_path?: string;
  }>>([])
  
  const [isLoading, setIsLoading] = useState(false)

  const loadAdminStatus = async () => {
    try {
      const admin = await runtimeClient.getAdminStatus()
      setAdminReady(admin.has_admin_key)
      setBackendUnlocked(admin.unlocked)
      if (!admin.unlocked) {
        setIsAdminUnlocked(false)
      }
    } catch (err) {
      console.error('Failed to load admin status:', err)
      setAdminReady(null)
      setBackendUnlocked(false)
      setIsAdminUnlocked(false)
    }
  }

  const loadHistory = async () => {
    try {
      const records = await runtimeClient.listIssuanceHistory()
      // Sort by latest first
      setHistory(records.sort((a, b) => b.issued_at - a.issued_at))
    } catch (err) {
      console.error('Failed to load history:', err)
      setHistory([])
    }
  }

  useEffect(() => {
    void loadAdminStatus()
    void loadHistory()
  }, [])

  useWailsEvent<{ has_admin_key?: boolean; unlocked?: boolean }>('admin:status', (payload) => {
    if (typeof payload?.has_admin_key === 'boolean') {
      setAdminReady(payload.has_admin_key)
    }
    if (typeof payload?.unlocked === 'boolean') {
      setBackendUnlocked(payload.unlocked)
      if (!payload.unlocked) {
        setIsAdminUnlocked(false)
      }
    }
  })

  useEffect(() => {
    if (!backendUnlocked && isAdminUnlocked) {
      setIsAdminUnlocked(false)
      setAdminPassphrase('')
      setAdminPasswordInput('')
      pushToast({
        title: 'Phiên quản trị hết hạn',
        description: 'Vui lòng đăng nhập lại để tiếp tục.',
        variant: 'destructive',
      })
    }
  }, [backendUnlocked, isAdminUnlocked, pushToast])

  const handleInit = async () => {
    if (!adminPasswordInput.trim()) return
    setIsLoading(true)
    try {
      await runtimeClient.initAdminKey(adminPasswordInput.trim())
      setAdminPassphrase(adminPasswordInput.trim())
      setBackendUnlocked(true)
      setIsAdminUnlocked(true)
      pushToast({
        title: 'Thành công',
        description: 'Đã khởi tạo mã quản trị gốc.',
        variant: 'default',
      })
      await loadAdminStatus()
    } catch (e) {
      pushToast({
        title: 'Lỗi khởi tạo',
        description: String(e),
        variant: 'destructive',
      })
    } finally {
      setIsLoading(false)
    }
  }

  const handleUnlock = async () => {
    if (!adminPasswordInput.trim()) return
    setIsLoading(true)
    try {
      await runtimeClient.verifyAdminPassphrase(adminPasswordInput.trim())
      setAdminPassphrase(adminPasswordInput.trim())
      setBackendUnlocked(true)
      setIsAdminUnlocked(true)
      pushToast({
        title: 'Đã mở khóa',
        description: 'Phiên quản trị có hiệu lực trong 15 phút.',
        variant: 'default',
      })
      await loadAdminStatus()
    } catch (e) {
      pushToast({
        title: 'Mật khẩu sai',
        description: 'Vui lòng kiểm tra lại mã quản trị của bạn.',
        variant: 'destructive',
      })
    } finally {
      setIsLoading(false)
    }
  }

  const handleRelock = () => {
    setIsAdminUnlocked(false)
    setAdminPassphrase('')
    setAdminPasswordInput('')
  }

  const handleIssue = async () => {
    if (!adminPassphrase || !displayName.trim() || !peerId.trim() || !mlsPublicKey.trim()) {
      pushToast({
        title: 'Thiếu thông tin',
        description: 'Vui lòng điền đầy đủ các trường yêu cầu.',
        variant: 'destructive',
      })
      return
    }

    setIsLoading(true)
    try {
      const savedPath = await runtimeClient.createBundle({
        display_name: displayName.trim(),
        peer_id: peerId.trim(),
        public_key_hex: mlsPublicKey.trim(),
        admin_passphrase: adminPassphrase,
      })

      if (savedPath) {
        pushToast({
          title: 'Đã cấp phát thành công',
          description: `Bundle được lưu tại: ${savedPath}`,
          variant: 'default',
        })
        setDisplayName('')
        setPeerId('')
        setMlsPublicKey('')
        await loadHistory()
      }
    } catch (e) {
      pushToast({
        title: 'Lỗi cấp phát',
        description: String(e),
        variant: 'destructive',
      })
    } finally {
      setIsLoading(false)
    }
  }

  const handleImportRequestFile: ChangeEventHandler<HTMLInputElement> = async (event) => {
    const file = event.target.files?.[0]
    if (!file) return
    
    try {
      const raw = await file.text()
      const req = await runtimeClient.parseDeviceRequestJson(raw)
      setPeerId(req.peer_id)
      setMlsPublicKey(req.mls_public_key)
      pushToast({
        title: 'Đã nạp file yêu cầu',
        description: `Thông tin từ ${file.name} đã được điền tự động.`,
        variant: 'default',
      })
    } catch (e) {
      pushToast({
        title: 'File không hợp lệ',
        description: 'Đảm bảo file JSON đúng định dạng yêu cầu.',
        variant: 'destructive',
      })
    } finally {
      event.target.value = ''
    }
  }

  const formatDate = (seconds: number) => {
    return new Date(seconds * 1000).toLocaleString('vi-VN', {
      day: '2-digit',
      month: '2-digit',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit'
    })
  }

  if (!isAdminUnlocked) {
    return (
      <div className="flex h-full items-center justify-center bg-slate-950/20 p-6 backdrop-blur-sm">
        <Card className="w-full max-w-[420px] border-slate-800 bg-slate-900/60 shadow-2xl backdrop-blur-md">
          <CardHeader className="space-y-1 text-center">
            <div className="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-emerald-500/10 ring-1 ring-emerald-500/20">
              <ShieldCheck className="h-7 w-7 text-emerald-500" />
            </div>
            <CardTitle className="text-2xl font-bold tracking-tight text-slate-100">Xác thực Quản trị</CardTitle>
            <CardDescription className="text-balance text-slate-400">
              Nhập mã quản trị để truy cập các tính năng bảo mật nội bộ.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="passphrase" className="text-slate-300">Mã quản trị (PIN/Password)</Label>
              <div className="relative group">
                <Lock className="absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500 transition-colors group-focus-within:text-emerald-500" />
                <Input
                  id="passphrase"
                  type="password"
                  value={adminPasswordInput}
                  onChange={(e) => setAdminPasswordInput(e.target.value)}
                  placeholder="••••••••"
                  className="bg-slate-950/50 border-slate-700 pl-10 text-slate-100 placeholder:text-slate-600 focus:border-emerald-500/50 focus:ring-emerald-500/20 transition-all"
                  onKeyDown={(e) => e.key === 'Enter' && handleUnlock()}
                  autoFocus
                />
              </div>
            </div>
          </CardContent>
          <CardFooter className="flex flex-col gap-3">
            <div className="flex w-full gap-3">
              {adminReady === false ? (
                <Button 
                  className="flex-1 bg-slate-100 text-slate-950 hover:bg-slate-200 font-semibold" 
                  onClick={handleInit}
                  disabled={!adminPasswordInput.trim() || isLoading}
                >
                  Khởi tạo
                </Button>
              ) : null}
              <Button 
                className="flex-1 bg-emerald-600 hover:bg-emerald-500 text-white font-semibold shadow-lg shadow-emerald-900/20 transition-all" 
                onClick={handleUnlock}
                disabled={!adminPasswordInput.trim() || isLoading}
              >
                {isLoading ? 'Đang mở...' : 'Mở khóa'}
              </Button>
            </div>
          </CardFooter>
        </Card>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col bg-slate-950/10 p-4 md:p-8 space-y-8 overflow-y-auto custom-scrollbar">
      {/* Header Section */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between border-b border-slate-800/60 pb-8">
        <div className="space-y-1.5">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-emerald-500/10 ring-1 ring-emerald-500/20">
              <ShieldCheck className="h-4.5 w-4.5 text-emerald-500" />
            </div>
            <h1 className="text-2xl font-bold tracking-tight text-slate-100">Bảng điều khiển Quản trị</h1>
          </div>
          <div className="flex items-center gap-3 text-xs">
            <div className="flex items-center gap-1.5 rounded-full bg-emerald-500/10 px-2.5 py-0.5 font-medium text-emerald-400 border border-emerald-500/20">
              <span className="relative flex h-1.5 w-1.5">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-1.5 w-1.5 bg-emerald-500"></span>
              </span>
              PHIÊN HOẠT ĐỘNG
            </div>
            <div className="flex items-center gap-1.5 text-slate-400">
              <Clock className="h-3.5 w-3.5" />
              <span>Thời hạn: <span className="font-mono text-slate-200">15 phút</span></span>
            </div>
          </div>
        </div>
        <Button 
          variant="outline" 
          size="sm" 
          className="border-slate-800 bg-slate-900/40 text-slate-400 hover:bg-slate-800 hover:text-slate-100 transition-colors"
          onClick={handleRelock}
        >
          <LogOut className="mr-2 h-4 w-4" />
          Khóa phiên làm việc
        </Button>
      </div>

      <div className="grid grid-cols-1 gap-8 xl:grid-cols-12 h-full items-start">
        {/* Issuance Workspace */}
        <div className="xl:col-span-7 space-y-8">
          <Card className="border-slate-800 bg-slate-900/40 shadow-xl backdrop-blur-sm overflow-hidden">
            <CardHeader className="pb-6 border-b border-slate-800/40 bg-slate-900/20">
              <div className="flex items-center gap-2.5 text-emerald-500">
                <UserPlus className="h-5 w-5" />
                <CardTitle className="text-lg">Cấp phát danh tính mới (PKI)</CardTitle>
              </div>
              <CardDescription className="text-slate-400 text-xs mt-1">
                Tạo và ký bundle xác thực để cho phép người dùng tham gia mạng lưới.
              </CardDescription>
            </CardHeader>
            <CardContent className="pt-8 space-y-8">
              {/* Tab Selector */}
              <div className="flex p-1 rounded-xl bg-slate-950/60 border border-slate-800/60 w-fit mx-auto sm:mx-0">
                <button
                  onClick={() => setActiveTab('manual')}
                  className={cn(
                    "flex items-center gap-2 px-6 py-2 text-xs font-semibold rounded-lg transition-all",
                    activeTab === 'manual' 
                      ? "bg-emerald-600 text-white shadow-lg shadow-emerald-900/20" 
                      : "text-slate-400 hover:bg-slate-800 hover:text-slate-200"
                  )}
                >
                  <Key className="h-3.5 w-3.5" />
                  Nhập thủ công
                </button>
                <button
                  onClick={() => setActiveTab('file')}
                  className={cn(
                    "flex items-center gap-2 px-6 py-2 text-xs font-semibold rounded-lg transition-all",
                    activeTab === 'file' 
                      ? "bg-emerald-600 text-white shadow-lg shadow-emerald-900/20" 
                      : "text-slate-400 hover:bg-slate-800 hover:text-slate-200"
                  )}
                >
                  <FileBadge className="h-3.5 w-3.5" />
                  Nhập từ file
                </button>
              </div>

              {/* Dynamic Content */}
              <div className="grid gap-6 animate-in fade-in duration-300">
                <div className="grid gap-6 sm:grid-cols-2">
                  <div className="space-y-2.5">
                    <Label className="text-[11px] font-bold uppercase tracking-wider text-slate-500 ml-1">Tên người dùng</Label>
                    <div className="relative group">
                      <User className="absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500 transition-colors group-focus-within:text-emerald-500" />
                      <Input
                        value={displayName}
                        onChange={(e) => setDisplayName(e.target.value)}
                        placeholder="VD: Nguyễn Văn A"
                        className="bg-slate-950/40 border-slate-700/60 pl-10 text-xs h-10 focus:ring-emerald-500/10"
                      />
                    </div>
                  </div>
                  
                  {activeTab === 'manual' && (
                    <div className="space-y-2.5">
                      <Label className="text-[11px] font-bold uppercase tracking-wider text-slate-500 ml-1">Peer ID (libp2p)</Label>
                      <div className="relative group">
                        <Fingerprint className="absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500 transition-colors group-focus-within:text-emerald-500" />
                        <Input
                          value={peerId}
                          onChange={(e) => setPeerId(e.target.value)}
                          placeholder="12D3KooW..."
                          className="bg-slate-950/40 border-slate-700/60 pl-10 text-xs h-10 font-mono focus:ring-emerald-500/10"
                        />
                      </div>
                    </div>
                  )}
                  
                  {activeTab === 'file' && (
                    <div className="space-y-2.5">
                      <Label className="text-[11px] font-bold uppercase tracking-wider text-slate-500 ml-1">Tệp yêu cầu (.json)</Label>
                      <div className="flex gap-2">
                        <div className="relative flex-1">
                          <Input 
                            type="file" 
                            accept=".json,application/json" 
                            onChange={(e) => void handleImportRequestFile(e)} 
                            className="bg-slate-950/40 border-slate-700/60 text-[10px] h-10 file:bg-emerald-500/10 file:text-emerald-400 file:border-0 file:rounded-md file:mr-4 file:px-3 file:py-1 file:font-semibold hover:file:bg-emerald-500/20"
                          />
                        </div>
                      </div>
                    </div>
                  )}
                </div>

                {activeTab === 'manual' && (
                  <div className="space-y-2.5">
                    <Label className="text-[11px] font-bold uppercase tracking-wider text-slate-500 ml-1">MLS Public Key (Hex)</Label>
                    <div className="relative group">
                      <ShieldAlert className="absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-500 transition-colors group-focus-within:text-emerald-500" />
                      <Input
                        value={mlsPublicKey}
                        onChange={(e) => setMlsPublicKey(e.target.value)}
                        placeholder="0x..."
                        className="bg-slate-950/40 border-slate-700/60 pl-10 text-xs h-10 font-mono focus:ring-emerald-500/10"
                      />
                    </div>
                  </div>
                )}
                
                {activeTab === 'file' && peerId && (
                  <div className="rounded-xl bg-emerald-500/5 border border-emerald-500/20 p-4 space-y-3 animate-in zoom-in-95 duration-200">
                    <div className="flex items-center gap-2 text-emerald-400">
                      <CheckCircle2 className="h-4 w-4" />
                      <span className="text-[11px] font-bold uppercase tracking-widest">Dữ liệu đã trích xuất</span>
                    </div>
                    <div className="grid gap-3 text-[11px]">
                      <div className="flex justify-between">
                        <span className="text-slate-500">Peer ID:</span>
                        <span className="text-slate-200 font-mono">{peerId}</span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-slate-500">MLS Key:</span>
                        <span className="text-slate-200 font-mono truncate max-w-[200px]">{mlsPublicKey}</span>
                      </div>
                    </div>
                  </div>
                )}
              </div>
            </CardContent>
            
            <CardFooter className="bg-slate-900/20 border-t border-slate-800/40 p-6">
              <Button
                className="w-full h-11 bg-emerald-600 hover:bg-emerald-500 text-white font-bold transition-all shadow-lg shadow-emerald-900/20 hover:scale-[1.01] active:scale-[0.99]"
                onClick={handleIssue}
                disabled={isLoading || !backendUnlocked || !adminPassphrase || !displayName.trim() || !peerId.trim() || !mlsPublicKey.trim()}
              >
                {isLoading ? (
                  <div className="flex items-center gap-2">
                    <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/20 border-t-white" />
                    Đang xử lý...
                  </div>
                ) : (
                  <>
                    <Lock className="mr-2 h-4 w-4" />
                    Ký và Cấp phát Bundle
                  </>
                )}
              </Button>
            </CardFooter>
          </Card>
        </div>

        {/* History Panel */}
        <div className="xl:col-span-5 h-full min-h-[500px]">
          <Card className="flex flex-col h-full border-slate-800 bg-slate-900/40 shadow-xl backdrop-blur-sm overflow-hidden">
            <CardHeader className="pb-4 border-b border-slate-800/40">
              <div className="flex items-center gap-2.5 text-slate-300">
                <History className="h-5 w-5" />
                <CardTitle className="text-lg">Lịch sử cấp phát</CardTitle>
              </div>
              <CardDescription className="text-slate-400 text-xs mt-1">
                Các bản ghi cấp phát danh tính gần đây.
              </CardDescription>
            </CardHeader>
            <CardContent className="flex-1 p-0 overflow-hidden">
              <div className="h-full overflow-y-auto custom-scrollbar">
                {history.length === 0 ? (
                  <div className="flex flex-col items-center justify-center h-full py-24 text-slate-600 opacity-40">
                    <History className="h-12 w-12 mb-4" />
                    <p className="text-sm font-medium italic">Chưa có bản ghi nào</p>
                  </div>
                ) : (
                  <div className="divide-y divide-slate-800/50">
                    {history.map((entry) => (
                      <div key={entry.id} className="group p-5 hover:bg-slate-800/30 transition-all cursor-default">
                        <div className="flex items-start justify-between gap-4">
                          <div className="space-y-1.5 min-w-0">
                            <div className="flex items-center gap-2">
                              <p className="text-sm font-bold text-slate-100 truncate">{entry.display_name}</p>
                              <div className="h-1.5 w-1.5 rounded-full bg-emerald-500/40" />
                            </div>
                            <div className="flex items-center gap-1.5 text-[11px] text-slate-500 font-mono">
                              <Fingerprint className="h-3.5 w-3.5 opacity-60" />
                              <span className="truncate">{entry.peer_id}</span>
                            </div>
                          </div>
                          <div className="flex flex-col items-end gap-1.5 shrink-0">
                            <span className="text-[10px] font-bold text-slate-500 bg-slate-950/50 px-2 py-0.5 rounded border border-slate-800/60 uppercase tracking-tighter">
                              {formatDate(entry.issued_at)}
                            </span>
                            {entry.bundle_path && (
                              <div className="flex items-center gap-1 text-[10px] text-emerald-500/70 group-hover:text-emerald-400 transition-colors">
                                <FileUp className="h-3 w-3" />
                                <span>Đã lưu file</span>
                              </div>
                            )}
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}
