#!/usr/bin/env ruby
# frozen_string_literal: true

keys = %w[
  PORT HOST DATABASE_URL API_KEY JWT_SECRET QUOTED_VALUE SPACED_VALUE
  EQUALS_VALUE MULTILINE_VALUE VITE_PUBLIC_TITLE NEXT_PUBLIC_TITLE
]
failed = false
keys.each do |key|
  unless ENV.key?(key)
    puts "POGO_ENV\t#{key}\tMISSING"
    failed = true
    next
  end
  puts "POGO_ENV\t#{key}\t#{ENV[key].unpack1('H*')}"
end
exit(failed ? 1 : 0)
