FROM ubuntu

ARG YT_DLP_VERSION=2026.08.19
ARG GALLERY_DL_VERSION=v1.32.14
ARG DENO_VERSION=v2.9.7

RUN apt update && apt upgrade -y && apt install -y unzip curl ffmpeg

RUN mkdir -p /root/.local/bin && \
    curl -fL https://github.com/yt-dlp/yt-dlp/releases/download/${YT_DLP_VERSION}/yt-dlp -o /root/.local/bin/yt-dlp && \
    curl -fL https://codeberg.org/mikf/gallery-dl/releases/download/${GALLERY_DL_VERSION}/gallery-dl.bin -o /root/.local/bin/gallery-dl && \
    chmod 0755 /root/.local/bin/yt-dlp /root/.local/bin/gallery-dl
RUN curl -fsSL https://deno.land/install.sh | sh -s ${DENO_VERSION}

RUN echo 'export PATH="/root/.local/bin:/root/.deno/bin:$PATH"' >> ~/.bashrc

COPY app.out /app/app
RUN chmod 0755 /app/app
WORKDIR /app

ENV PATH="/root/.local/bin:/root/.deno/bin:$PATH"

ENTRYPOINT ["/app/app"]
