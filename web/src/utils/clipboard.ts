/** Writes text to the system clipboard. Returns whether this likely succeeded. */
export async function copyTextToClipboard(text: string): Promise<boolean> {
  if (!text) return false
  if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // continue to fallback
    }
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.setAttribute('readonly', '')
    ta.style.position = 'fixed'
    ta.style.left = '-9999px'
    ta.style.top = '0'
    document.body.appendChild(ta)
    ta.focus()
    ta.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  } catch {
    return false
  }
}

/** For success toasts after showing a one-time password. */
export async function toastDetailAfterPasswordClipboard(password: string): Promise<string> {
  const ok = await copyTextToClipboard(password)
  return ok
    ? 'Einmalpasswort wurde in die Zwischenablage kopiert.'
    : 'Einmalpasswort konnte nicht automatisch kopiert werden — bitte abschreiben oder manuell kopieren.'
}
