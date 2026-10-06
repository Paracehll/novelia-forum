import type { WebKit } from '@novelia/web-kit';

/**
 * Session view shared with web-kit. Module-level because Pinia stores build
 * their cache keys at instantiation time and are also created outside any
 * component setup (`router.ts`, `main.ts`), where `useWebKit()` cannot inject.
 * Components should keep calling `useWebKit()` directly.
 */
export let whoami: WebKit['whoami'];

export function initializeSession(kit: WebKit) {
  whoami = kit.whoami;
}
