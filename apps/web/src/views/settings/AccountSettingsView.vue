<script setup lang="ts">
import { BlockOutlined } from '@vicons/material';
import { RouterLink, RouterView, useRoute } from 'vue-router';

const route = useRoute();
</script>

<template>
  <div class="page-container py-4 md:py-8">
    <div class="mx-auto max-w-6xl">
      <h1 class="sr-only">账户设置</h1>

      <div
        class="grid min-w-0 gap-6 md:grid-cols-[15rem_minmax(0,1fr)] md:gap-8"
      >
        <aside class="min-w-0">
          <nav aria-label="账户设置栏目" class="md:sticky md:top-6">
            <p class="mb-2 px-3 text-xs font-semibold text-muted">内容与互动</p>
            <RouterLink
              :to="{ name: 'blacklist' }"
              :aria-current="route.name === 'blacklist' ? 'page' : undefined"
              class="relative flex items-center gap-2 rounded-md px-3 py-2 text-sm text-ink transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
              :class="
                route.name === 'blacklist'
                  ? 'bg-primary-soft font-semibold'
                  : 'hover:bg-embedded'
              "
            >
              <span
                v-if="route.name === 'blacklist'"
                class="absolute inset-y-2 -left-1 w-1 rounded-full bg-primary"
                aria-hidden="true"
              />
              <BlockOutlined
                class="size-5 shrink-0 text-muted"
                aria-hidden="true"
              />
              黑名单管理
            </RouterLink>
          </nav>
        </aside>

        <div class="min-w-0">
          <RouterView />
        </div>
      </div>
    </div>
  </div>
</template>
