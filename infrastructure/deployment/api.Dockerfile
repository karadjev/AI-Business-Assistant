# Artifact-only, offline build. Input is prepared and hashed by package.py.
FROM scratch
COPY api worker migrate healthcheck runtime-entrypoint /app/
COPY ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
USER 10001:10001
WORKDIR /app
ENTRYPOINT ["/app/runtime-entrypoint"]
CMD ["api"]
