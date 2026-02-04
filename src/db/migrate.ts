import 'dotenv/config';
import { drizzle } from 'drizzle-orm/node-postgres';
import { migrate } from 'drizzle-orm/node-postgres/migrator';
import pg from 'pg';

const databaseUrl = process.env['DATABASE_URL'];
if (!databaseUrl) {
  console.error('DATABASE_URL environment variable is required');
  process.exit(1);
}

async function runMigrations() {
  console.log('Connecting to database...');
  const pool = new pg.Pool({ connectionString: databaseUrl });
  const db = drizzle(pool);

  console.log('Running migrations from drizzle/ ...');
  await migrate(db, { migrationsFolder: './drizzle' });
  console.log('Migrations applied successfully.');

  await pool.end();
}

runMigrations().catch((err) => {
  console.error('Migration failed:', err);
  process.exit(1);
});
