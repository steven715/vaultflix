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
  const area = document.createElement('textarea')
  area.value = text
  area.setAttribute('readonly', '')
  area.style.position = 'fixed'
  area.style.opacity = '0'
  document.body.appendChild(area)
  area.focus()
  area.select()
  try {
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    area.remove()
  }
}
