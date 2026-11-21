# Match the existing glibc native modules; do not place them on Alpine/musl.
# Only Node and its shared libraries enter the final image, not browsers/tools.
ARG NODE_IMAGE=mcr.microsoft.com/playwright:v1.64.0-noble
FROM ${NODE_IMAGE} AS runtime
RUN mkdir -p /node-runtime/usr/bin /node-runtime/lib/x86_64-linux-gnu /node-runtime/lib64 /node-runtime/etc/ssl/certs && \
    cp /usr/bin/node /node-runtime/usr/bin/node && \
    for lib in libdl.so.2 libstdc++.so.6 libm.so.6 libgcc_s.so.1 libpthread.so.0 libc.so.6 libresolv.so.2 librt.so.1; do \
      cp -L /lib/x86_64-linux-gnu/$lib /node-runtime/lib/x86_64-linux-gnu/$lib; done && \
    cp -L /lib64/ld-linux-x86-64.so.2 /node-runtime/lib64/ld-linux-x86-64.so.2 && \
    cp /etc/ssl/certs/ca-certificates.crt /node-runtime/etc/ssl/certs/ca-certificates.crt
FROM scratch
COPY --from=runtime /node-runtime/ /
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 HOSTNAME=0.0.0.0 PORT=3000
WORKDIR /apps/web
COPY --chown=10001:10001 web/ ./
COPY artifacts.cjs /guard/artifacts.cjs
COPY web-entrypoint.cjs /guard/web-entrypoint.cjs
USER 10001:10001
ENTRYPOINT ["/usr/bin/node", "/guard/web-entrypoint.cjs"]
