import tailwindcss from '@tailwindcss/vite';
import vue from '@vitejs/plugin-vue';
import { execFileSync } from 'node:child_process';
import { fileURLToPath, URL } from 'node:url';
import { defineConfig, loadEnv, type UserConfig } from 'vite';

const repositoryRoot = fileURLToPath(new URL('../..', import.meta.url));

function readGitValue(args: string[]) {
  return execFileSync('git', args, {
    cwd: repositoryRoot,
    encoding: 'utf8',
  }).trim();
}

function setupAuthProxy(config: UserConfig) {
  const authUrl = 'https://auth.novelia.cc';
  const proxy = config.server!.proxy!;

  proxy['^/api(?!/v1/auth)'] = proxy['/api'];
  delete proxy['/api'];

  proxy['/api/v1/auth'] = {
    target: authUrl,
    changeOrigin: true,
  };

  proxy['/auth-proxy/assets'] = {
    target: authUrl,
    changeOrigin: true,
    rewrite: (path) => path.replace(/^\/auth-proxy\/assets/, '/assets'),
  };

  proxy['/auth-proxy'] = {
    target: authUrl,
    changeOrigin: true,
    rewrite: (path) => path.replace(/^\/auth-proxy/, ''),
    selfHandleResponse: true,
    headers: { 'accept-encoding': 'identity' },
    configure(proxyServer) {
      proxyServer.on('proxyRes', (proxyResponse, _request, response) => {
        const chunks: Buffer[] = [];
        proxyResponse.on('data', (chunk: Buffer) => chunks.push(chunk));
        proxyResponse.on('end', () => {
          const body = Buffer.concat(chunks)
            .toString()
            .replaceAll('/assets', '/auth-proxy/assets');

          response.statusCode = proxyResponse.statusCode ?? 200;
          response.statusMessage = proxyResponse.statusMessage ?? '';
          for (const [key, value] of Object.entries(proxyResponse.headers)) {
            if (value !== undefined) response.setHeader(key, value);
          }
          response.removeHeader('content-length');
          response.end(body);
        });
      });
    },
  };
}

export default defineConfig(({ command, mode }) => {
  const env = loadEnv(
    mode,
    fileURLToPath(new URL('.', import.meta.url)),
    'VITE_',
  );
  const isServe = command === 'serve';
  const commitSha = isServe
    ? readGitValue(['rev-parse', 'HEAD'])
    : env.VITE_COMMIT_SHA || 'unknown';
  const buildTime = isServe
    ? readGitValue(['show', '-s', '--format=%cI', 'HEAD'])
    : env.VITE_BUILD_TIME || new Date().toISOString();
  const apiMode = env.VITE_API_MODE;
  const apiUrl =
    apiMode === 'native'
      ? 'http://localhost:8080'
      : apiMode === 'local'
        ? 'http://localhost:5000'
        : 'https://forum.novelia.cc';

  const config: UserConfig = {
    define: {
      __BUILD_TIME__: JSON.stringify(buildTime),
      __COMMIT_SHA__: JSON.stringify(commitSha),
      __AUTH_URL__: JSON.stringify(
        isServe ? '/auth-proxy/' : 'https://auth.novelia.cc',
      ),
    },
    plugins: [vue(), tailwindcss()],
    build: {
      cssCodeSplit: false,
      rolldownOptions: {
        output: {
          manualChunks: (id) =>
            id.replaceAll('\\', '/').includes('/node_modules/')
              ? 'vendor'
              : undefined,
        },
      },
    },
    optimizeDeps: {
      exclude: ['@novelia/web-kit'],
    },
    server: {
      port: 5173,
      proxy: {
        '/api': {
          target: apiUrl,
          changeOrigin: true,
          rewrite:
            apiMode === 'native'
              ? (path) => path.replace(/^\/api/, '')
              : undefined,
        },
      },
    },
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
  };

  if (isServe) setupAuthProxy(config);

  return config;
});
