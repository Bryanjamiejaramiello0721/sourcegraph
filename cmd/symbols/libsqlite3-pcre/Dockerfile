FROM alpine:3.10@sha256:acd3ca9941a85e8ed16515bfc5328e4e2f8c128caa72959a58a127b7801ee01f

# hadolint ignore=DL3003,DL3018,DL4006
RUN apk --no-cache add --virtual build-deps curl git gcc make libc-dev pcre-dev sqlite-dev && \
  mkdir /sqlite3-pcre && \
  curl -fsSL https://codeload.github.com/ralight/sqlite3-pcre/tar.gz/c98da412b431edb4db22d3245c99e6c198d49f7a | tar -C /sqlite3-pcre -xzvf - --strip 1 && \
  cd /sqlite3-pcre && \
  make && \
  apk --no-cache --purge del build-deps
