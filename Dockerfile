FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Copy binary from build context
# (Build with: make nix-build && cp result/bin/lake-writer .)
COPY lake-writer /usr/local/bin/lake-writer
RUN chmod +x /usr/local/bin/lake-writer

# Create non-root user
RUN useradd -m -u 1000 obsrvr
USER obsrvr

# Expose ports
EXPOSE 50099 8088

# Set working directory
WORKDIR /home/obsrvr

# Default command
ENTRYPOINT ["/usr/local/bin/lake-writer"]
CMD ["-config", "/config/production.yaml"]
