export default async function globalSetup(): Promise<void> {
  if (process.env.SCANKIT_FAIL_SETUP === '1') {
    throw new Error('the ledger database is down');
  }
}
