#!/bin/sh
# Print env values as hex so spaces, quotes, and newlines survive the pipe.
keys="PORT HOST DATABASE_URL API_KEY JWT_SECRET QUOTED_VALUE SPACED_VALUE EQUALS_VALUE MULTILINE_VALUE VITE_PUBLIC_TITLE NEXT_PUBLIC_TITLE"
fail=0
for key in $keys; do
  eval "value=\${$key-}"
  if eval "[ -z \"\${$key+x}\" ]"; then
    printf 'POGO_ENV\t%s\tMISSING\n' "$key"
    fail=1
    continue
  fi
  hex=$(printf '%s' "$value" | od -An -tx1 | tr -d ' \n')
  printf 'POGO_ENV\t%s\t%s\n' "$key" "$hex"
done
exit "$fail"
