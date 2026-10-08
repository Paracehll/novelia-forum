<script setup lang="ts">
import { computed } from 'vue';

import { renderMarkdown, type MarkdownMode } from '@novelia/forum-api';

const props = defineProps<{
  mode: MarkdownMode;
  source: string;
}>();

const renderedContent = computed(() =>
  renderMarkdown(props.source, props.mode),
);

function spoilerFromTarget(target: EventTarget | null) {
  return target instanceof Element
    ? target.closest<HTMLElement>('[data-markdown-spoiler]')
    : null;
}

function toggleSpoiler(spoilerElement: HTMLElement) {
  spoilerElement.dataset.hide =
    spoilerElement.dataset.hide === 'true' ? 'false' : 'true';
}

function handleClick(event: MouseEvent) {
  const spoilerElement = spoilerFromTarget(event.target);
  if (!spoilerElement) return;
  if (
    spoilerElement.dataset.hide === 'false' &&
    event.target instanceof Element &&
    event.target.closest('a')
  ) {
    return;
  }
  if (spoilerElement.dataset.hide === 'true') event.preventDefault();
  toggleSpoiler(spoilerElement);
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' && event.key !== ' ') return;
  const spoilerElement = spoilerFromTarget(event.target);
  if (!spoilerElement) return;
  event.preventDefault();
  toggleSpoiler(spoilerElement);
}
</script>

<template>
  <div
    class="markdown-content"
    :class="`markdown-content--${mode}`"
    v-html="renderedContent"
    @click="handleClick"
    @keydown="handleKeydown"
  />
</template>

<style scoped>
.markdown-content {
  color: var(--color-ink);
  font-size: 0.9375rem;
  line-height: 1.75rem;
  overflow-wrap: anywhere;
}

.markdown-content--article {
  --markdown-heading-margin: 1.25em 0.5em;
  --markdown-block-margin: 0 0.75em;
  line-height: 1.7;
}

.markdown-content--comment {
  font-size: 0.875rem;
  /* 与小说站前端一致：Naive UI common.lineHeight */
  line-height: 1.6;
}

.markdown-content :deep(> :first-child) {
  margin-top: 0;
}

.markdown-content :deep(> :last-child) {
  margin-bottom: 0;
}

.markdown-content :deep(h1),
.markdown-content :deep(h2),
.markdown-content :deep(h3),
.markdown-content :deep(h4) {
  margin-block: var(--markdown-heading-margin, 1.6em 0.65em);
  color: var(--color-ink);
  font-weight: 700;
  line-height: 1.3;
}

.markdown-content :deep(h1) {
  padding-bottom: 0.35em;
  border-bottom: 1px solid var(--color-divider);
  font-size: 1.5em;
}

.markdown-content :deep(h2) {
  padding-bottom: 0.3em;
  border-bottom: 1px solid var(--color-divider);
  font-size: 1.3em;
}

.markdown-content :deep(h3) {
  font-size: 1.15em;
}

.markdown-content :deep(h4) {
  font-size: 1em;
}

.markdown-content :deep(h1),
.markdown-content :deep(h2),
.markdown-content :deep(h3),
.markdown-content :deep(h4),
.markdown-content :deep(h5),
.markdown-content :deep(h6) {
  scroll-margin-top: 5rem;
}

.markdown-content :deep(p),
.markdown-content :deep(ul),
.markdown-content :deep(ol),
.markdown-content :deep(blockquote),
.markdown-content :deep(pre),
.markdown-content :deep(table) {
  margin-block: var(--markdown-block-margin, 0 1em);
}

.markdown-content :deep(ul),
.markdown-content :deep(ol) {
  padding-left: 1.5rem;
}

.markdown-content :deep(ul) {
  list-style: disc;
}

.markdown-content :deep(ol) {
  list-style: decimal;
}

.markdown-content :deep(li + li) {
  margin-top: 0.25em;
}

.markdown-content :deep(a) {
  color: var(--color-primary);
  text-decoration: underline;
  text-decoration-color: color-mix(
    in srgb,
    var(--color-primary) 45%,
    transparent
  );
  text-underline-offset: 0.2em;
}

.markdown-content :deep(a:hover) {
  color: var(--color-primary-hover);
}

.markdown-content :deep(blockquote) {
  border-left: 3px solid var(--color-primary);
  padding: 0.25rem 0 0.25rem 1rem;
  color: var(--color-muted);
}

.markdown-content :deep(code) {
  border-radius: 0.25rem;
  background: var(--color-divider);
  padding: 0.15em 0.35em;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.88em;
}

.markdown-content :deep(pre) {
  overflow-x: auto;
  border: 1px solid var(--color-divider);
  border-radius: 0.375rem;
  background: var(--color-divider);
  padding: 1rem;
  line-height: 1.6;
}

.markdown-content :deep(pre code) {
  border-radius: 0;
  background: transparent;
  padding: 0;
  font-size: 0.85em;
}

.markdown-content :deep(hr) {
  margin-block: 1.5rem;
  border: 0;
  border-top: 1px solid var(--color-divider);
}

.markdown-content :deep(table) {
  display: block;
  max-width: 100%;
  overflow-x: auto;
  border-collapse: collapse;
}

.markdown-content :deep(th),
.markdown-content :deep(td) {
  border: 1px solid var(--color-border);
  padding: 0.4rem 0.7rem;
  text-align: left;
}

.markdown-content :deep(th) {
  background: var(--color-paper);
  font-weight: 600;
}

.markdown-content :deep(img) {
  max-width: 100%;
  height: auto;
  border-radius: 0.375rem;
}

.markdown-content :deep(details) {
  margin-bottom: 1rem;
  border: 1px solid var(--color-border);
  border-radius: 0.375rem;
  padding: 0.65rem 0.85rem;
}

.markdown-content :deep(summary) {
  cursor: pointer;
  font-weight: 600;
}

.markdown-content :deep(details[open] summary) {
  margin-bottom: 0.75rem;
}

.markdown-content :deep(.markdown-star-rating) {
  display: inline-flex;
  flex-wrap: nowrap;
  margin-bottom: 1rem;
}

.markdown-content :deep(.markdown-star) {
  position: relative;
  display: flex;
  width: 20px;
  height: 20px;
  color: rgb(219, 219, 223);
}

.markdown-content :deep(.markdown-star:not(:first-child)) {
  margin-left: 6px;
}

.markdown-content :deep(.markdown-star::before),
.markdown-content :deep(.markdown-star__half::before) {
  width: 20px;
  height: 20px;
  background-color: currentColor;
  content: '';
  -webkit-mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 512 512'%3E%3Cpath d='M394 480a16 16 0 01-9.39-3L256 383.76 127.39 477a16 16 0 01-24.55-18.08L153 310.35 23 221.2a16 16 0 019-29.2h160.38l48.4-148.95a16 16 0 0130.44 0l48.4 149H480a16 16 0 019.05 29.2L359 310.35l50.13 148.53A16 16 0 01394 480z'/%3E%3C/svg%3E")
    center / contain no-repeat;
  mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 512 512'%3E%3Cpath d='M394 480a16 16 0 01-9.39-3L256 383.76 127.39 477a16 16 0 01-24.55-18.08L153 310.35 23 221.2a16 16 0 019-29.2h160.38l48.4-148.95a16 16 0 0130.44 0l48.4 149H480a16 16 0 019.05 29.2L359 310.35l50.13 148.53A16 16 0 01394 480z'/%3E%3C/svg%3E")
    center / contain no-repeat;
}

.markdown-content :deep(.markdown-star--active) {
  color: #4fb233;
}

.markdown-content :deep(.markdown-star__half) {
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  display: flex;
  width: 50%;
  overflow: hidden;
  color: transparent;
}

.markdown-content :deep(.markdown-star__half--active) {
  color: #4fb233;
}

.markdown-content :deep(.markdown-star__half::before) {
  flex: 0 0 20px;
}

.markdown-content :deep([data-markdown-spoiler]) {
  cursor: pointer;
  border-radius: 0.2em;
  background: var(--color-spoiler);
  padding: 0.05em 0.25em;
  transition:
    background 150ms ease,
    color 150ms ease;
}

.markdown-content :deep([data-markdown-spoiler][data-hide='true']),
.markdown-content :deep([data-markdown-spoiler][data-hide='true'] *) {
  color: transparent !important;
}

.markdown-content :deep([data-markdown-spoiler][data-hide='false']),
.markdown-content :deep([data-markdown-spoiler][data-hide='false'] *) {
  color: var(--color-spoiler-ink);
}
</style>
