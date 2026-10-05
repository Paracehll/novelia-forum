<script setup lang="ts">
import { FavoriteBorderOutlined, FavoriteFilled } from '@vicons/material';
import { XButton } from '@novelia/web-kit';

import { authUser } from '@/api';

withDefaults(
  defineProps<{
    favorited: boolean;
    loading: boolean;
    prominent?: boolean;
  }>(),
  { prominent: false },
);

defineEmits<{ toggle: [] }>();
</script>

<template>
  <XButton
    v-if="authUser"
    :variant="
      prominent
        ? favorited
          ? 'outline'
          : 'primary'
        : favorited
          ? 'ghost-active'
          : 'ghost'
    "
    size="sm"
    :disabled="loading"
    :title="favorited ? '取消收藏' : '收藏帖子，稍后阅读'"
    :aria-label="favorited ? '取消收藏帖子' : '收藏帖子'"
    :aria-pressed="favorited"
    :aria-busy="loading"
    @click="$emit('toggle')"
  >
    <FavoriteFilled v-if="favorited" class="size-4" aria-hidden="true" />
    <FavoriteBorderOutlined v-else class="size-4" aria-hidden="true" />
    {{ loading ? '处理中…' : favorited ? '取消收藏' : '收藏' }}
  </XButton>
</template>
