export const loaded = Boolean(process.env.SCANKIT_NEVER);

if (!loaded) {
  throw new Error('the archive cannot be opened');
}
