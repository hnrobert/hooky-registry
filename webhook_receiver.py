#!/usr/bin/env python3
"""
Simple Registry webhook receiver that uses the Docker SDK to pull images
and restart containers that are running from the pushed image.

It expects Docker socket to be mounted into the container (already configured
in the compose file).
"""
import json
import logging
import os
from http.server import BaseHTTPRequestHandler, HTTPServer

try:
    import docker
except Exception:
    docker = None

PORT = int(os.environ.get('WEBHOOK_PORT', '5001'))
REGISTRY = os.environ.get('REGISTRY', '127.0.0.1:5000')

logging.basicConfig(
    level=logging.INFO,
    format='[%(asctime)s] %(levelname)s %(message)s',
)

if docker:
    client = docker.from_env()
else:
    client = None


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get('content-length', 0))
        body = self.rfile.read(length)
        try:
            event = json.loads(body)
        except Exception:
            self.send_response(400)
            self.end_headers()
            return

        if not client:
            logging.error("Docker SDK not available")
            self.send_response(500)
            self.end_headers()
            return

        # Basic handling for push events
        for target in event.get('events', []):
            if target.get('action') != 'push':
                continue
            repo = target.get('target', {}).get('repository')
            tag = target.get('target', {}).get('tag', 'latest')
            image = f"{REGISTRY}/{repo}:{tag}"
            logging.info(f"Detected push: {image}")
            try:
                img = client.images.pull(image)
                logging.info(
                    f"Pulled image: {image} -> {getattr(img, 'id', '<no-id>')}"
                )

                containers = client.containers.list(
                    filters={"ancestor": image})
                if not containers:
                    logging.info(f"No running containers found for {image}")
                for c in containers:
                    logging.info(f"Restarting container {c.name}")
                    c.restart()
            except Exception as e:
                logging.error(f"Error handling image {image}: {e}")

        self.send_response(200)
        self.end_headers()


if __name__ == '__main__':
    logging.info(f"Webhook receiver listening on 0.0.0.0:{PORT}, registry={REGISTRY}")
    server = HTTPServer(('0.0.0.0', PORT), Handler)
    server.serve_forever()
