<script setup lang="ts">
import { LockOutlined, PushPinOutlined } from '@vicons/material';

import type { PostTag } from '@/api';

defineProps<{
  tags: PostTag[];
  pinned?: boolean;
  locked?: boolean;
  categoryName?: string;
}>();

const tagColors = [
  'bg-primary-soft text-primary',
  'bg-info-soft text-info',
  'bg-warning-soft text-warning',
  'bg-purple-50 text-purple-600',
  'bg-error-soft text-error-strong',
];

function tagClass(color: number) {
  return tagColors[Math.abs(color) % tagColors.length];
}
</script>

<template>
  <div
    v-if="categoryName || pinned || locked || tags.length"
    class="flex flex-wrap items-center gap-1.5 text-xs"
  >
    <span v-if="categoryName" class="font-medium text-primary">
      {{ categoryName }}
    </span>
    <span
      v-if="pinned"
      class="inline-flex size-5 items-center justify-center rounded-sm bg-warning-soft text-warning"
      title="已置顶"
      aria-label="已置顶"
    >
      <PushPinOutlined class="size-3.5" aria-hidden="true" />
    </span>
    <span
      v-if="locked"
      class="inline-flex size-5 items-center justify-center rounded-sm bg-warning-soft text-warning"
      title="评论区已锁定"
      aria-label="评论区已锁定"
    >
      <LockOutlined class="size-3.5" aria-hidden="true" />
    </span>
    <span
      v-for="tag in tags"
      :key="tag.id"
      class="rounded-sm px-2 py-0.5 font-medium"
      :class="tagClass(tag.color)"
    >
      {{ tag.name }}
    </span>
  </div>
</template>
