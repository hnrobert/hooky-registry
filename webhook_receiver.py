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
# Strategy for updating containers: 'recreate' or 'restart'
UPDATE_STRATEGY = os.environ.get('UPDATE_STRATEGY', 'recreate').lower()

logging.basicConfig(
    level=logging.INFO,
    format='[%(asctime)s] %(levelname)s %(message)s',
)

if docker:
    client = docker.from_env()
else:
    client = None


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        # Health check endpoint
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            health_status = {
                'status': 'healthy',
                'docker_available': client is not None,
                'registry': REGISTRY,
                'port': PORT,
                'update_strategy': UPDATE_STRATEGY
            }
            self.wfile.write(json.dumps(health_status).encode())
            return

        # Default response for other GET requests
        self.send_response(404)
        self.end_headers()

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
                # Force pull the latest version of the image
                logging.info(f"Force pulling latest image: {image}")
                img = client.images.pull(image, platform=None)
                logging.info(
                    f"Pulled image: {image} -> {getattr(img, 'id', '<no-id>')}"
                )

                containers = client.containers.list(
                    filters={"ancestor": image})
                if not containers:
                    logging.info(f"No running containers found for {image}")

                # Choose update strategy
                if UPDATE_STRATEGY == 'recreate':
                    self._recreate_containers(containers, image)
                else:
                    self._restart_containers(containers)

            except Exception as e:
                logging.error(f"Error handling image {image}: {e}")

        self.send_response(200)
        self.end_headers()

    def _restart_containers(self, containers):
        """Simple restart strategy - just restart containers (may not use new image)"""
        for c in containers:
            try:
                logging.info(
                    f"Restarting container {c.name} with ID {c.id[:12]}")
                c.restart()
                logging.info(f"Successfully restarted container {c.name}")
            except Exception as e:
                logging.error(f"Failed to restart container {c.name}: {e}")

    def _recreate_containers(self, containers, image):
        """Recreate containers to ensure they use the latest image"""
        if not client:
            logging.error(
                "Docker client not available for container recreation")
            return

        for c in containers:
            logging.info(f"Recreating container {c.name} with ID {c.id[:12]}")

            try:
                # Get container configuration
                container_info = client.api.inspect_container(c.id)
                config = container_info['Config']
                host_config = container_info['HostConfig']

                # Extract important configuration
                container_name = c.name
                env_vars = config.get('Env', [])
                ports = {}
                volumes = {}

                # Extract port mappings
                if host_config.get('PortBindings'):
                    for container_port, host_binding in host_config['PortBindings'].items():
                        if host_binding:
                            host_port = host_binding[0]['HostPort']
                            ports[container_port] = host_port

                # Extract volume mappings
                if host_config.get('Binds'):
                    for bind in host_config['Binds']:
                        parts = bind.split(':')
                        if len(parts) >= 2:
                            host_path, container_path = parts[0], parts[1]
                            mode = parts[2] if len(parts) > 2 else 'rw'
                            volumes[host_path] = {
                                'bind': container_path, 'mode': mode}

                # Extract network settings
                networks = list(container_info.get(
                    'NetworkSettings', {}).get('Networks', {}).keys())

                # Extract restart policy
                restart_policy = host_config.get('RestartPolicy', {})

                # Extract working directory
                working_dir = config.get('WorkingDir', '')

                # Extract command and entrypoint
                cmd = config.get('Cmd')
                entrypoint = config.get('Entrypoint')

                # Stop and remove the old container
                logging.info(f"Stopping container {container_name}")
                c.stop(timeout=10)
                logging.info(f"Removing container {container_name}")
                c.remove()

                # Create and start new container with the same configuration
                logging.info(
                    f"Creating new container {container_name} with updated image")

                # Prepare run arguments
                run_args = {
                    'image': image,
                    'name': container_name,
                    'environment': env_vars,
                    'detach': True,
                    'remove': False
                }

                if ports:
                    run_args['ports'] = ports
                if volumes:
                    run_args['volumes'] = volumes
                if restart_policy:
                    run_args['restart_policy'] = restart_policy
                if working_dir:
                    run_args['working_dir'] = working_dir
                if cmd:
                    run_args['command'] = cmd
                if entrypoint:
                    run_args['entrypoint'] = entrypoint

                if client:
                    new_container = client.containers.run(**run_args)
                else:
                    logging.error("Docker client not available")
                    return

                # Connect to networks if needed
                for network_name in networks:
                    if network_name != 'bridge':  # Skip default bridge network
                        try:
                            if client:
                                network = client.networks.get(network_name)
                                network.connect(new_container)
                                logging.info(
                                    f"Connected container to network {network_name}")
                        except Exception as net_e:
                            logging.warning(
                                f"Failed to connect to network {network_name}: {net_e}")

                logging.info(
                    f"Successfully recreated container {container_name} with new image")

            except Exception as container_error:
                logging.error(
                    f"Failed to recreate container {c.name}: {container_error}")
                # Try to restart the original container if recreation failed
                try:
                    c.restart()
                    logging.info(
                        f"Fallback: restarted original container {c.name}")
                except Exception as restart_error:
                    logging.error(
                        f"Failed to restart original container {c.name}: {restart_error}")


if __name__ == '__main__':
    logging.info(
        f"Webhook receiver listening on 0.0.0.0:{PORT}, registry={REGISTRY}")
    logging.info(f"Update strategy: {UPDATE_STRATEGY}")
    server = HTTPServer(('0.0.0.0', PORT), Handler)
    server.serve_forever()
