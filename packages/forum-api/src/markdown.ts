import { container } from '@mdit/plugin-container';
import { spoiler } from '@mdit/plugin-spoiler';
import DOMPurify, { type Config } from 'dompurify';
import MarkdownIt, { type RendererRule, type Token } from 'markdown-it';
import anchor from 'markdown-it-anchor';

export type MarkdownMode = 'article' | 'comment';

const COMMENT_DISABLED_RULES = [
  'blockquote',
  'code',
  'fence',
  'heading',
  'hr',
  'image',
  'lheading',
  'reference',
  'table',
];

const STAR_COUNT = 5;

function parseStarValue(info: string) {
  if (!/^\d+(?:\.\d+)?$/.test(info)) return 0;
  return Math.min(STAR_COUNT, Number(info));
}

/** Reads the text after a container marker, e.g. `点击展开` in `::: details 点击展开`. */
function containerParams(token: Token, name: string) {
  return token.info.trim().slice(name.length).trim();
}

function renderStarRating(value: number) {
  const rating = Math.round(value * 2) / 2;
  const stars = Array.from({ length: STAR_COUNT }, (_, index) => {
    const active = index + 1 <= rating;
    const halfActive = !active && index + 0.5 <= rating;
    return `<span class="markdown-star${active ? ' markdown-star--active' : ''}"><span class="markdown-star__half${halfActive ? ' markdown-star__half--active' : ''}"></span></span>`;
  }).join('');

  return `<div class="markdown-star-rating" role="img" aria-label="评分 ${rating} / ${STAR_COUNT}">${stars}</div>\n`;
}

function createMarkdown(mode: MarkdownMode) {
  const markdown = new MarkdownIt({
    html: false,
    linkify: true,
    breaks: true,
  });
  markdown.use(spoiler, {
    tag: 'span',
    attrs: [
      ['data-markdown-spoiler', ''],
      ['data-hide', 'true'],
      ['role', 'button'],
      ['tabindex', '0'],
    ],
  });
  markdown.use(container, {
    name: 'details',
    openRenderer: (tokens: Token[], index: number): string => {
      const summary = markdown.utils.escapeHtml(
        containerParams(tokens[index], 'details') || '点击展开',
      );
      return `<details><summary>${summary}</summary>\n`;
    },
    closeRenderer: () => '</details>\n',
  });
  markdown.use(container, {
    name: 'star',
    openRenderer: (tokens: Token[], index: number): string => {
      const info = containerParams(tokens[index], 'star');
      return renderStarRating(parseStarValue(info));
    },
    closeRenderer: () => '',
  });

  if (mode === 'article') {
    markdown.use(anchor, {
      // Keep Unicode IDs so existing percent-encoded Chinese fragments match.
      slugify: (title: string) => title.trim().toLowerCase().replace(/\s+/g, '-'),
    });
  } else {
    markdown.disable(COMMENT_DISABLED_RULES);
  }

  const defaultLinkOpen: RendererRule =
    markdown.renderer.rules.link_open ??
    ((tokens, index, options, _env, renderer) =>
      renderer.renderToken(tokens, index, options));
  const renderExternalLink: RendererRule = (
    tokens,
    index,
    options,
    env,
    renderer,
  ) => {
    const href = tokens[index].attrGet('href');
    if (typeof href === 'string' && !href.startsWith('#')) {
      tokens[index].attrSet('target', '_blank');
      tokens[index].attrSet('rel', 'noopener noreferrer');
    }
    return defaultLinkOpen(tokens, index, options, env, renderer);
  };
  markdown.renderer.rules.link_open = renderExternalLink;

  return markdown;
}

const markdownByMode: Record<
  MarkdownMode,
  ReturnType<typeof createMarkdown>
> = {
  article: createMarkdown('article'),
  comment: createMarkdown('comment'),
};

const sanitizeConfig = {
  USE_PROFILES: { html: true },
  ADD_ATTR: ['target'],
  FORBID_TAGS: ['style'],
  FORBID_ATTR: ['style'],
} satisfies Config;

/** Renders and sanitizes forum Markdown for safe insertion into the DOM. */
export function renderMarkdown(source: string, mode: MarkdownMode) {
  return DOMPurify.sanitize(
    markdownByMode[mode].render(source),
    sanitizeConfig,
  );
}
