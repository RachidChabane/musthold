import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    include: ['test/**/*.test.ts'],
    globalSetup: ['./test/global-setup.ts'],
    browser: process.env.SCANKIT_BROWSER === '1' ? { enabled: true, instances: [{ browser: 'chromium' }] } : undefined,
  },
});
