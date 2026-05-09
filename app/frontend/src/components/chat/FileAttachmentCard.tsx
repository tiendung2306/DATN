import { Archive, FileAudio, FileImage, FileSpreadsheet, FileText, FileVideo, File as FileIcon, Download, Loader2, CheckCircle2 } from 'lucide-react'
import { Button } from '../ui/button'
import { FileAttachment, fileIconByMimeOrExt, formatFileSize } from '../../lib/chatModel'

interface FileAttachmentCardProps {
  file: FileAttachment
  isMine: boolean
  state?: 'idle' | 'downloading' | 'completed' | 'failed'
  localPath?: string
  onDownload?: () => void
  onOpenFile?: () => void
  disabled?: boolean
}

function IconForType({ type }: { type: ReturnType<typeof fileIconByMimeOrExt> }) {
  switch (type) {
    case 'pdf':
      return <FileText className="h-4 w-4 text-rose-300" />
    case 'doc':
      return <FileText className="h-4 w-4 text-sky-300" />
    case 'sheet':
      return <FileSpreadsheet className="h-4 w-4 text-emerald-300" />
    case 'archive':
      return <Archive className="h-4 w-4 text-amber-300" />
    case 'image':
      return <FileImage className="h-4 w-4 text-fuchsia-300" />
    case 'video':
      return <FileVideo className="h-4 w-4 text-indigo-300" />
    case 'audio':
      return <FileAudio className="h-4 w-4 text-cyan-300" />
    default:
      return <FileIcon className="h-4 w-4 text-slate-300" />
  }
}

export default function FileAttachmentCard({
  file,
  isMine,
  state = 'idle',
  localPath,
  onDownload,
  onOpenFile,
  disabled,
}: FileAttachmentCardProps) {
  const iconType = fileIconByMimeOrExt(file)
  const canDownload = !isMine && Boolean(onDownload)
  const shortHash = file.sha256.length > 12 ? `${file.sha256.slice(0, 12)}...` : file.sha256

  return (
    <div className="mt-2 rounded-xl border border-slate-700/70 bg-slate-900/70 p-3 backdrop-blur-[1px]">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 rounded-lg border border-slate-700 bg-slate-800 p-2">
          <IconForType type={iconType} />
        </div>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium text-slate-100">{file.name}</p>
          <div className="mt-1 flex flex-wrap items-center gap-1.5 text-[11px] text-slate-400">
            <span className="rounded border border-slate-700 px-1.5 py-0.5">{formatFileSize(file.size)}</span>
            <span className="rounded border border-slate-700 px-1.5 py-0.5">{file.mime_type}</span>
            <span className="rounded border border-slate-700 px-1.5 py-0.5">#{shortHash}</span>
          </div>
          <p className="mt-1 text-[11px] text-slate-500">Mã hóa đầu cuối · MLS exporter</p>
          {state === 'completed' && localPath ? (
            <p className="mt-1 truncate text-[11px] text-emerald-300">Đã lưu: {localPath}</p>
          ) : null}
          {state === 'failed' ? (
            <p className="mt-1 text-[11px] text-rose-300">Tải xuống thất bại. Vui lòng thử lại.</p>
          ) : null}
        </div>
      </div>
      <div className="mt-3 flex items-center justify-end gap-2">
        {!isMine && onOpenFile ? (
          <Button
            size="xs"
            variant="ghost"
            className="text-slate-200"
            onClick={onOpenFile}
            disabled={disabled || state === 'downloading'}
          >
            Mở file
          </Button>
        ) : null}
        {canDownload ? (
          <Button
            size="xs"
            variant={state === 'completed' ? 'secondary' : state === 'failed' ? 'destructive' : 'default'}
            className={state === 'completed' ? '' : state === 'failed' ? '' : 'bg-emerald-500 text-slate-900 hover:bg-emerald-400'}
            onClick={onDownload}
            disabled={disabled || state === 'downloading'}
          >
            {state === 'downloading' ? (
              <>
                <Loader2 className="mr-1 h-3 w-3 animate-spin" />
                Đang tải
              </>
            ) : state === 'completed' ? (
              <>
                <CheckCircle2 className="mr-1 h-3 w-3" />
                Tải lại
              </>
            ) : (
              <>
                <Download className="mr-1 h-3 w-3" />
                {state === 'failed' ? 'Thử lại' : 'Tải xuống'}
              </>
            )}
          </Button>
        ) : (
          <span className="text-[11px] text-slate-500">Sẵn sàng cho thành viên khác tải xuống</span>
        )}
      </div>
    </div>
  )
}
