import { createWebKit } from '@novelia/web-kit';
import { PiniaColada } from '@pinia/colada';
import { createApp } from 'vue';

import App from './App.vue';
import { initializeApi } from './api';
import router from './router';
import { initializeSession } from './session';
import { pinia } from './stores';
import { useCategoryStore } from './stores/category';
import './styles.css';

const webKit = createWebKit({
  auth: {
    app: 'f',
    url: __AUTH_URL__,
    storageKey: 'forum:session:v1',
  },
  brand: '论坛',
  repository: {
    url: 'https://github.com/auto-novel/forum',
    buildTime: __BUILD_TIME__,
    commitSha: __COMMIT_SHA__,
  },
  themeStorageKey: 'forum:theme:v1',
});
initializeSession(webKit);
initializeApi(webKit.api);

createApp(App)
  .use(webKit)
  .use(pinia)
  .use(PiniaColada, {
    queryOptions: {
      staleTime: 60_000,
      gcTime: 5 * 60_000,
    },
  })
  .use(router)
  .mount('#app');

void useCategoryStore().initialize();
