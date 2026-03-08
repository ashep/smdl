FROM ubuntu

RUN apt update && apt upgrade -y && apt install -y unzip curl ffmpeg

RUN mkdir -p /root/.local/bin && \
    curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /root/.local/bin/yt-dlp && \
    curl -L https://github.com/mikf/gallery-dl/releases/download/v1.31.9/gallery-dl.bin -o /root/.local/bin/gallery-dl && \
    chmod 0755 /root/.local/bin/yt-dlp /root/.local/bin/gallery-dl
RUN curl -fsSL https://deno.land/install.sh | sh

RUN echo 'export PATH="/root/.local/bin:/root/.deno/bin:$PATH"' >> ~/.bashrc

COPY app.out /app/app
RUN chmod 0755 /app/app
WORKDIR /app

ENV PATH="/root/.local/bin:/root/.deno/bin:$PATH"

ENTRYPOINT ["/app/app"]
