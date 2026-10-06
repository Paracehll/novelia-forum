/**
 * 帖子分享所需的最小字段。这里刻意不依赖 `@/api` 类型，
 * 让分享逻辑可以在不引入接口层的情况下复用于详情页以外的场景。
 */
export interface ShareablePost {
  id: number;
  title: string;
}

/** 帖子的规范分享链接，与站内 `/p/:id` 路由保持一致。 */
export function postShareUrl(
  post: ShareablePost,
  origin = window.location.origin,
): string {
  return new URL(`/p/${post.id}`, origin).toString();
}

/** Markdown 引用文本，标题中的方括号需要转义，避免破坏链接语法。 */
export function postShareMarkdown(
  post: ShareablePost,
  origin = window.location.origin,
): string {
  const url = postShareUrl(post, origin);
  const title = post.title.trim().replace(/[[\]]/g, '\\$&');
  return title ? `[${title}](${url})` : url;
}

/** 复制文本到剪贴板，优先异步剪贴板接口，失败时回退到选区复制。 */
export async function copyText(text: string): Promise<void> {
  const clipboard = globalThis.navigator?.clipboard;
  if (clipboard?.writeText) {
    try {
      await clipboard.writeText(text);
      return;
    } catch {
      // 非安全上下文或权限被拒绝时，继续尝试选区复制。
    }
  }
  if (!copyTextBySelection(text)) throw new Error('复制到剪贴板失败');
}

function copyTextBySelection(text: string): boolean {
  if (typeof document === 'undefined') return false;
  const area = document.createElement('textarea');
  area.value = text;
  area.setAttribute('readonly', '');
  area.style.position = 'fixed';
  area.style.top = '0';
  area.style.opacity = '0';
  document.body.appendChild(area);
  try {
    area.select();
    return document.execCommand?.('copy') === true;
  } catch {
    return false;
  } finally {
    area.remove();
  }
}
