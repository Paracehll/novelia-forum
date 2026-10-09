/** Builds a Markdown link for a nonempty, single-line selection and an HTTP(S) URL. */
export function createMarkdownLink(
  selection: string,
  clipboardText: string,
): string | null {
  if (!selection || /[\r\n]/.test(selection)) return null;
  if (/http/i.test(selection)) return null;
  const pastedText = clipboardText.trim();
  if (!/^https?:\/\/\S+$/i.test(pastedText)) return null;

  let url: URL;
  try {
    url = new URL(pastedText);
  } catch {
    return null;
  }
  if (!url.hostname || !['http:', 'https:'].includes(url.protocol)) return null;

  const label = selection.replace(/[\\[\]]/g, '\\$&');
  const destination = url.href.replace(/[()]/g, (character) =>
    character === '(' ? '%28' : '%29',
  );
  return `[${label}](${destination})`;
}

/**
 * Turns a URL pasted over selected text into a Markdown link.
 * Returns true only after insertion succeeds and the native paste is prevented.
 * Unsupported insertion falls back to native paste to preserve browser undo behavior.
 */
export function handleMarkdownLinkPaste(
  event: ClipboardEvent,
  element: HTMLTextAreaElement | null | undefined,
): boolean {
  if (
    event.defaultPrevented ||
    !element ||
    element.disabled ||
    element.readOnly
  )
    return false;
  const { selectionStart, selectionEnd, value } = element;
  const clipboardText = event.clipboardData?.getData('text/plain');
  if (!clipboardText || selectionStart === selectionEnd) return false;

  const link = createMarkdownLink(
    value.slice(selectionStart, selectionEnd),
    clipboardText,
  );
  if (!link) return false;
  if (
    element.maxLength >= 0 &&
    value.length - (selectionEnd - selectionStart) + link.length >
      element.maxLength
  )
    return false;

  const document = element.ownerDocument;
  // insertText acts on the focused element, so never mutate another editor.
  if (document.activeElement !== element) return false;
  let inputFired = false;
  const markInput = () => {
    inputFired = true;
  };
  element.addEventListener('input', markInput, { once: true });
  let inserted = false;
  try {
    inserted = document.execCommand('insertText', false, link);
  } catch {
    return false;
  } finally {
    element.removeEventListener('input', markInput);
  }
  if (!inserted) return false;

  event.preventDefault();
  if (!inputFired) {
    const EventConstructor = document.defaultView?.Event ?? Event;
    element.dispatchEvent(new EventConstructor('input', { bubbles: true }));
  }
  return true;
}
