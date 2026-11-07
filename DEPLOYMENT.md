# Public deployment

This project has no database: quiz scores and missed words are kept in browser memory. The word dataset is mounted read-only from the server, and Rektion data is included in the app image. There is no SQLite file to persist or back up.

## Requirements

- A Linux server with Docker Engine and the Docker Compose plugin.
- A public hostname pointing to the server. A free dynamic DNS hostname works; the hostname must resolve publicly to this server.
- Inbound TCP ports 80 and 443 allowed in the server firewall/provider firewall. Do not expose port 8080 publicly.
- `data/words.jsonl` present on the server.

## Secure SSH access first

The root password shared in chat is exposed. Change it before proceeding. Create a non-root deployment account, install your SSH public key for that account, and disable root password SSH login. Do not send passwords in chat or store them in this project.

## Deploy

1. Copy this project to a directory on the server, including `data/words.jsonl`. Do not copy `web/node_modules`, `web/dist`, or `data/sources`.
2. In the project directory, create `.env` from `.env.example` and set `PUBLIC_HOST` to the free hostname you configured to point at the server.
3. Start the services:

   ```sh
   docker compose up -d --build
   ```

4. Check startup logs and health:

   ```sh
   docker compose logs -f app caddy
   curl -fsS https://YOUR_PUBLIC_HOST/api/health
   ```

Open `https://YOUR_PUBLIC_HOST`. Caddy obtains and renews the certificate automatically when public DNS points to the server and ports 80/443 are reachable.

## Update

Copy changed source files to the server and run `docker compose up -d --build` again. The word JSONL file remains on the server and is mounted read-only into the container.
