#!/usr/bin/env python3
"""Loopback-only TCP relay to an existing isolated demo web, via Docker stdio.

No network/daemon changes, package installation, container port publication or
Docker socket mount. Run in the foreground; Ctrl+C closes the relay connections.
"""
import argparse
import json
import os
import socket
import socketserver
import subprocess
import threading

BRIDGE = '''const net=require('node:net');
const s=net.connect(3000,'127.0.0.1');
s.setTimeout(120000,()=>s.end());
s.on('error',()=>process.exit(1));
s.on('close',()=>process.exit(0));
process.stdin.pipe(s);s.pipe(process.stdout);
'''


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--container', default='aba-demo10d-local-web-1')
    parser.add_argument('--port', type=int, default=43010)
    args = parser.parse_args()
    if not 1024 <= args.port <= 65535:
        parser.error('port must be between 1024 and 65535')
    # Inspect only selected metadata; do not read or print container environment.
    info = json.loads(subprocess.check_output([
        'docker', 'inspect', '--format',
        '{"running":{{json .State.Running}},"service":{{json (index .Config.Labels "com.docker.compose.service")}},"project":{{json (index .Config.Labels "com.docker.compose.project")}},"networks":{{json .NetworkSettings.Networks}}}',
        args.container], text=True))
    if not info['running'] or info['service'] != 'web' or info['project'] != 'aba-demo10d-local':
        parser.error('requires running web service in dedicated aba-demo10d-local project')
    if not info['networks']:
        parser.error('demo network missing')
    for network in info['networks']:
        internal = subprocess.check_output([
            'docker', 'network', 'inspect', '--format', '{{.Internal}}', network], text=True).strip()
        if internal != 'true':
            parser.error('all demo web networks must remain internal')
    connections = set()
    lock = threading.Lock()

    class Relay(socketserver.BaseRequestHandler):
        def handle(self):
            client = self.request
            with lock:
                connections.add(client)
            proc = subprocess.Popen([
                'docker', 'exec', '-i', args.container, 'node', '-e', BRIDGE],
                stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)

            def upstream():
                try:
                    while data := client.recv(65536):
                        proc.stdin.write(data)
                        proc.stdin.flush()
                except (OSError, BrokenPipeError):
                    pass
                finally:
                    proc.stdin.close()

            reader = threading.Thread(target=upstream, daemon=True)
            reader.start()
            try:
                while data := os.read(proc.stdout.fileno(), 65536):
                    client.sendall(data)
            except OSError:
                pass
            finally:
                try:
                    client.shutdown(socket.SHUT_RDWR)
                except OSError:
                    pass
                reader.join()
                proc.stdout.close()
                proc.wait()  # stdin EOF closes the in-container bridge; no kill.
                with lock:
                    connections.discard(client)

    class Server(socketserver.ThreadingTCPServer):
        allow_reuse_address = True

    with Server(('127.0.0.1', args.port), Relay) as server:
        print(f'Demo relay http://127.0.0.1:{args.port} — Ctrl+C to stop', flush=True)
        try:
            server.serve_forever(poll_interval=0.2)
        except KeyboardInterrupt:
            with lock:
                for client in connections:
                    try:
                        client.shutdown(socket.SHUT_RDWR)
                    except OSError:
                        pass
    print('Demo relay stopped; containers and history preserved', flush=True)


if __name__ == '__main__':
    main()
