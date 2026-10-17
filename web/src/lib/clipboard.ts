// copyText puts text on the clipboard and resolves whether it did. The
// Clipboard API exists only in a secure context (https, localhost); over
// plain http on the LAN (ADR-0009) it is missing, so this falls back to
// copying from a hidden textarea.
export async function copyText(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // Denied or unavailable: try the fallback.
    }
  }
  return copyFromTextarea(text)
}

function copyFromTextarea(text: string): boolean {
  // The textarea borrows focus to select its text; give it back after.
  const focused = document.activeElement instanceof HTMLElement ? document.activeElement : null
  const area = document.createElement('textarea')
  area.value = text
  area.setAttribute('readonly', '') // no keyboard, no zoom on iOS
  Object.assign(area.style, { position: 'fixed', top: '0', left: '0', opacity: '0' })
  document.body.appendChild(area)
  area.focus({ preventScroll: true })
  area.select()
  area.setSelectionRange(0, text.length) // iOS Safari ignores select() on a readonly field
  try {
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    area.remove()
    focused?.focus({ preventScroll: true })
  }
}
