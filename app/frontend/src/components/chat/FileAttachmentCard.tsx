import { Archive, FileAudio, FileImage, FileSpreadsheet, FileText, FileVideo, File as FileIcon, Download, Loader2, CheckCircle2, ExternalLink, FolderOpen, Shield } from 'lucide-react'
import { Button } from '../ui/button'
import { FileAttachment, fileIconByMimeOrExt, formatFileSize, formatMimeType } from '../../lib/chatModel'

interface FileAttachmentCardProps {
  file: FileAttachment
  isMine: boolean
  state?: 'idle' | 'downloading' | 'completed' | 'failed'
  localPath?: string
  onDownload?: () => void
  onOpenFile?: () => void
  onOpenFileLocation?: () => void
  disabled?: boolean
}

function IconForType({ type }: { type: ReturnType<typeof fileIconByMimeOrExt> }) {
  switch (type) {
    case 'pdf':
      return <FileText className="h-5 w-5 text-rose-300" />
    case 'doc':
      return <FileText className="h-5 w-5 text-sky-300" />
    case 'sheet':
      return <FileSpreadsheet className="h-5 w-5 text-emerald-300" />
    case 'archive':
      return <Archive className="h-5 w-5 text-amber-300" />
    case 'image':
      return <FileImage className="h-5 w-5 text-fuchsia-300" />
    case 'video':
      return <FileVideo className="h-5 w-5 text-indigo-300" />
    case 'audio':
      return <FileAudio className="h-5 w-5 text-cyan-300" />
    default:
      return <FileIcon className="h-5 w-5 text-slate-300" />
  }
}

export default function FileAttachmentCard({
  file,
  isMine,
  state = 'idle',
  localPath,
  onDownload,
  onOpenFile,
  onOpenFileLocation,
  disabled,
}: FileAttachmentCardProps) {
  const iconType = fileIconByMimeOrExt(file)
  const canDownload = !isMine && Boolean(onDownload)

  const canOpenFile = Boolean(onOpenFile) && (isMine || state === 'completed')
  const canOpenFolder = Boolean(onOpenFileLocation) && (isMine || state === 'completed')

  return (
    <div className="mt-2 w-full max-w-[320px] rounded-xl border border-slate-700/70 bg-slate-900/70 p-3 backdrop-blur-[1px]">
      <div className="flex items-start gap-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-slate-700 bg-slate-800">
          <IconForType type={iconType} />
        </div>
        <div className="min-w-0 flex-1">
          <button
            type="button"
            onClick={canOpenFile ? onOpenFile : undefined}
            disabled={!canOpenFile || disabled}
            className={`group flex items-center gap-1.5 truncate text-sm font-medium text-left ${
              canOpenFile
                ? 'cursor-pointer text-slate-100 hover:text-emerald-300 hover:underline'
                : 'text-slate-100'
            }`}
            title={file.name}
          >
            <span className="truncate">{file.name}</span>
            {canOpenFile && (
              <ExternalLink className="h-3.5 w-3.5 shrink-0 opacity-0 transition-opacity group-hover:opacity-100" />
            )}
          </button>
          <div className="mt-1 flex items-center gap-2 text-[11px] text-slate-400">
            <span className="font-medium text-slate-300">{formatFileSize(file.size)}</span>
            <span className="h-3 w-px bg-slate-700" />
            <span>{formatMimeType(file.mime_type)}</span>
          </div>
        </div>
      </div>

      <div className="mt-3 flex items-center justify-between border-t border-slate-700/50 pt-2">
        <div className="flex items-center gap-1.5 text-[10px] text-slate-500">
          <Shield className="h-3 w-3 text-emerald-400" />
          <span>Mã hóa đầu cuối</span>
        </div>
        <div className="flex items-center gap-2">
          {state === 'completed' && localPath ? (
            <p className="max-w-[140px] truncate text-[10px] text-emerald-300" title={localPath}>Đã lưu</p>
          ) : null}
          {state === 'failed' ? (
            <p className="text-[10px] text-rose-300">Tải thất bại</p>
          ) : null}
          {canOpenFolder ? (
            <Button
              size="xs"
              variant="ghost"
              className="h-6 px-2 text-[11px] text-slate-200"
              onClick={onOpenFileLocation}
              disabled={disabled}
            >
              <FolderOpen className="mr-1 h-3 w-3" />
              Mở thư mục
            </Button>
          ) : null}
          {canDownload ? (
            <Button
              size="xs"
              variant={state === 'completed' ? 'secondary' : state === 'failed' ? 'destructive' : 'default'}
              className={`h-6 px-2 text-[11px] ${state === 'completed' ? '' : state === 'failed' ? '' : 'bg-emerald-500 text-slate-900 hover:bg-emerald-400'}`}
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
          ) : !isMine ? (
            <span className="text-[10px] text-slate-500">Chờ tải xuống</span>
          ) : null}
        </div>
      </div>
    </div>
  )
}
