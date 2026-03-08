FROM ubuntu

RUN apt update && apt upgrade -y && apt install -y unzip curl ffmpeg yt-dlp gallery-dl
RUN curl -fsSL https://deno.land/install.sh | sh
RUN echo 'export PATH="$HOME/.deno/bin:$PATH"' >> ~/.bashrc

COPY app.out /app/app
RUN chmod 0755 /app/app
WORKDIR /app

ENTRYPOINT ["/app/app"]
