/**
 * Client-side avatar pipeline: allow larger user picks, compress to backend limits.
 * Must stay aligned with store.MaxAvatarBytes (256 KiB) on the Go side.
 */

export const AVATAR_INPUT_MAX_BYTES = 5 * 1024 * 1024
export const AVATAR_OUTPUT_MAX_BYTES = 256 * 1024
/** Long edge cap after resize (no upscale). */
export const AVATAR_MAX_DIMENSION = 512
/** Reject decode if width*height exceeds this (e.g. huge BMP inside small file). */
export const AVATAR_MAX_PIXELS = 24 * 1024 * 1024

const WEBP_QUALITIES = [0.86, 0.78, 0.7, 0.62, 0.54, 0.46]
const JPEG_QUALITIES = [0.82, 0.72, 0.62, 0.52, 0.42]
const DIMENSION_STEPS = [512, 448, 384, 320, 256]

const ALLOWED_MIME = new Set(['image/png', 'image/jpeg', 'image/webp'])
const ALLOWED_EXT = new Set(['png', 'jpg', 'jpeg', 'webp'])

export class AvatarImageError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'AvatarImageError'
  }
}

function extFromName(name: string): string {
  const base = name.trim().toLowerCase().split(/[/\\]/).pop() ?? ''
  const i = base.lastIndexOf('.')
  return i >= 0 ? base.slice(i + 1) : ''
}

export function validateAvatarFile(file: File): void {
  if (!file || file.size <= 0) {
    throw new AvatarImageError('Chưa chọn file ảnh.')
  }
  if (file.size > AVATAR_INPUT_MAX_BYTES) {
    throw new AvatarImageError(`Ảnh gốc tối đa ${AVATAR_INPUT_MAX_BYTES / (1024 * 1024)} MiB.`)
  }
  const mime = (file.type || '').trim().toLowerCase()
  const ext = extFromName(file.name)
  if (mime && ALLOWED_MIME.has(mime)) return
  if (ext && ALLOWED_EXT.has(ext)) return
  throw new AvatarImageError('Chỉ hỗ trợ PNG, JPEG hoặc WebP.')
}

function loadImageViaObjectUrl(file: File): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file)
    const img = new Image()
    img.onload = () => {
      URL.revokeObjectURL(url)
      resolve(img)
    }
    img.onerror = () => {
      URL.revokeObjectURL(url)
      reject(new AvatarImageError('Không đọc được ảnh (file hỏng hoặc định dạng không hỗ trợ).'))
    }
    img.src = url
  })
}

type DecodedSource = {
  width: number
  height: number
  draw: (ctx: CanvasRenderingContext2D, tw: number, th: number) => void
  dispose: () => void
}

async function decodeToSource(file: File): Promise<DecodedSource> {
  if (typeof createImageBitmap === 'function') {
    try {
      const bitmap = await createImageBitmap(file)
      const w = bitmap.width
      const h = bitmap.height
      if (w <= 0 || h <= 0) {
        bitmap.close()
        throw new AvatarImageError('Kích thước ảnh không hợp lệ.')
      }
      if (w * h > AVATAR_MAX_PIXELS) {
        bitmap.close()
        throw new AvatarImageError(
          `Ảnh quá lớn (${w}×${h} px). Vui lòng chọn ảnh nhỏ hơn hoặc giảm độ phân giải trước.`,
        )
      }
      return {
        width: w,
        height: h,
        draw: (ctx, tw, th) => {
          ctx.drawImage(bitmap, 0, 0, tw, th)
        },
        dispose: () => {
          try {
            bitmap.close()
          } catch {
            /* ignore */
          }
        },
      }
    } catch (e) {
      if (e instanceof AvatarImageError) throw e
      // fall through to Image()
    }
  }
  const img = await loadImageViaObjectUrl(file)
  const w = img.naturalWidth || img.width
  const h = img.naturalHeight || img.height
  if (w <= 0 || h <= 0) {
    throw new AvatarImageError('Kích thước ảnh không hợp lệ.')
  }
  if (w * h > AVATAR_MAX_PIXELS) {
    throw new AvatarImageError(
      `Ảnh quá lớn (${w}×${h} px). Vui lòng chọn ảnh nhỏ hơn hoặc giảm độ phân giải trước.`,
    )
  }
  return {
    width: w,
    height: h,
    draw: (ctx, tw, th) => {
      ctx.drawImage(img, 0, 0, tw, th)
    },
    dispose: () => {},
  }
}

function scaleToMaxDimension(srcW: number, srcH: number, maxDim: number): { w: number; h: number } {
  const long = Math.max(srcW, srcH)
  if (long <= 0) return { w: 1, h: 1 }
  if (long <= maxDim) return { w: srcW, h: srcH }
  const scale = maxDim / long
  return {
    w: Math.max(1, Math.round(srcW * scale)),
    h: Math.max(1, Math.round(srcH * scale)),
  }
}

function canvasToBlob(canvas: HTMLCanvasElement, mime: string, quality: number): Promise<Blob | null> {
  return new Promise((resolve) => {
    canvas.toBlob((b) => resolve(b), mime, quality)
  })
}

async function tryEncodeUnderLimit(
  canvas: HTMLCanvasElement,
  preferWebp: boolean,
): Promise<{ blob: Blob; mime: string } | null> {
  const tryWebp = async (): Promise<{ blob: Blob; mime: string } | null> => {
    for (const q of WEBP_QUALITIES) {
      const blob = await canvasToBlob(canvas, 'image/webp', q)
      if (blob && blob.size > 0 && blob.size <= AVATAR_OUTPUT_MAX_BYTES) {
        return { blob, mime: 'image/webp' }
      }
    }
    return null
  }
  const tryJpeg = async (): Promise<{ blob: Blob; mime: string } | null> => {
    for (const q of JPEG_QUALITIES) {
      const blob = await canvasToBlob(canvas, 'image/jpeg', q)
      if (blob && blob.size > 0 && blob.size <= AVATAR_OUTPUT_MAX_BYTES) {
        return { blob, mime: 'image/jpeg' }
      }
    }
    return null
  }

  if (preferWebp) {
    const w = await tryWebp()
    if (w) return w
  }
  const j = await tryJpeg()
  if (j) return j
  if (!preferWebp) {
    const w = await tryWebp()
    if (w) return w
  }
  return null
}

export interface CompressedAvatarResult {
  bytes: number[]
  blob: Blob
  mime: string
  originalBytes: number
  outputBytes: number
  width: number
  height: number
  wasCompressed: boolean
}

/**
 * Resize + compress avatar for Wails upload. Throws AvatarImageError on failure.
 */
export async function compressAvatarFile(file: File): Promise<CompressedAvatarResult> {
  validateAvatarFile(file)
  const originalBytes = file.size
  const source = await decodeToSource(file)
  let lastError: Error | null = null

  try {
    for (const maxDim of DIMENSION_STEPS) {
      const { w: tw, h: th } = scaleToMaxDimension(source.width, source.height, maxDim)

      const canvas = document.createElement('canvas')
      canvas.width = tw
      canvas.height = th
      const ctx = canvas.getContext('2d')
      if (!ctx) {
        throw new AvatarImageError('Trình duyệt không hỗ trợ xử lý ảnh (canvas).')
      }

      // JPEG has no alpha: neutral background for JPEG fallback
      ctx.fillStyle = '#0f172a'
      ctx.fillRect(0, 0, tw, th)
      source.draw(ctx, tw, th)

      let preferWebp = true
      const probe = await canvasToBlob(canvas, 'image/webp', 0.5)
      preferWebp = Boolean(probe && probe.size > 0)

      const encoded = await tryEncodeUnderLimit(canvas, preferWebp)
      if (encoded) {
        const buf = await encoded.blob.arrayBuffer()
        const u8 = new Uint8Array(buf)
        const bytes = Array.from(u8)
        const wasCompressed = originalBytes !== u8.byteLength || source.width !== tw || source.height !== th
        return {
          bytes,
          blob: encoded.blob,
          mime: encoded.mime,
          originalBytes,
          outputBytes: u8.byteLength,
          width: tw,
          height: th,
          wasCompressed,
        }
      }
      lastError = new AvatarImageError('Không nén được ảnh dưới giới hạn 256 KiB.')
    }

    throw lastError ?? new AvatarImageError('Không nén được ảnh. Hãy thử ảnh khác hoặc giảm độ phân giải.')
  } finally {
    source.dispose()
  }
}

export function formatBytesShort(n: number): string {
  if (!Number.isFinite(n) || n < 0) return '0 B'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(n >= 10 * 1024 ? 0 : 1)} KiB`
  return `${(n / (1024 * 1024)).toFixed(1)} MiB`
}
