/** Längste Kante, auf die Handyfotos vor dem Hochladen verkleinert werden (lesbar, aber deutlich kleiner). */
const MAX_EDGE = 2000
const JPEG_QUALITY = 0.82

/**
 * Verkleinert Fotos vor dem Upload zu JPEG (Handyfotos haben oft 4–8 MB). PDFs und Dateien,
 * die der Browser nicht dekodieren kann, bleiben unverändert.
 */
export async function prepareReceiptFile(file: File): Promise<File> {
  if (!file.type.startsWith('image/') || file.type === 'image/gif') return file
  try {
    const bitmap = await createImageBitmap(file)
    const scale = Math.min(1, MAX_EDGE / Math.max(bitmap.width, bitmap.height))
    const w = Math.round(bitmap.width * scale)
    const h = Math.round(bitmap.height * scale)
    const canvas = document.createElement('canvas')
    canvas.width = w
    canvas.height = h
    const ctx = canvas.getContext('2d')
    if (!ctx) return file
    ctx.drawImage(bitmap, 0, 0, w, h)
    bitmap.close()
    const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/jpeg', JPEG_QUALITY))
    if (!blob || blob.size >= file.size) return file
    const base = file.name.replace(/\.[^.]+$/, '') || 'Beleg'
    return new File([blob], `${base}.jpg`, { type: 'image/jpeg' })
  } catch {
    return file
  }
}
