# frozen_string_literal: true

environment ENV.fetch('RACK_ENV', 'production')

workers Integer(ENV.fetch('WEB_CONCURRENCY', 4))

threads_count = Integer(ENV.fetch('PUMA_THREADS', 1))
threads threads_count, threads_count

preload_app!

port ENV.fetch('PORT', 8080)
bind "tcp://0.0.0.0:#{ENV.fetch('PORT', 8080)}"
