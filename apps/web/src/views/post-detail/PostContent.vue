<script setup lang="ts">
import { ChatBubbleOutlineOutlined } from '@vicons/material';
import { XButton } from '@novelia/web-kit';
import type { Post } from '@/api';
import MarkdownContent from '@/components/markdown/MarkdownContent.vue';

defineProps<{
  post: Post;
}>();
defineEmits<{ comments: [] }>();

function formatDate(value: string) {
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value));
}
</script>

<template>
  <article class="pb-6">
    <h1
      class="text-2xl leading-tight font-bold tracking-tight text-ink sm:text-3xl"
    >
      {{ post.title }}
    </h1>

    <div
      class="mt-5 flex flex-wrap items-center justify-between gap-3 text-xs text-muted"
    >
      <div class="flex items-center gap-2">
        <span class="font-medium text-ink/80">{{ post.authorUsername }}</span>
        <span aria-hidden="true">·</span>
        <time :datetime="post.createdAt">{{ formatDate(post.createdAt) }}</time>
      </div>
      <div class="flex items-center gap-4" aria-label="帖子数据">
        <span>{{ post.viewsCount }} 次浏览</span>
        <span>{{ post.commentsCount }} 条评论</span>
      </div>
    </div>

    <div
      class="mt-4 flex flex-wrap items-center gap-2"
      aria-label="帖子快捷操作"
    >
      <slot name="favorite" />
      <XButton
        as="a"
        href="#comments"
        variant="outline"
        size="sm"
        aria-controls="comments"
        @click.prevent="$emit('comments')"
      >
        <ChatBubbleOutlineOutlined class="size-4" aria-hidden="true" />
        评论
      </XButton>
      <slot name="more" />
    </div>
    <div class="my-6 h-px bg-divider" />
    <MarkdownContent mode="article" :source="post.content" />
  </article>
</template>
