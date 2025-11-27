# frozen_string_literal: true

set :database, {
  adapter: 'mysql2',
  pool: 100,
  pool_timeout: 10, # Wait up to 10 seconds for available connection
  host: ENV.fetch('ISHOCON_DB_HOST', '127.0.0.1'),
  username: ENV.fetch('ISHOCON_DB_USER', 'ishocon'),
  password: ENV.fetch('ISHOCON_DB_PASSWORD', 'ishocon'),
  database: ENV.fetch('ISHOCON_DB_NAME', 'ishocon3')
}
