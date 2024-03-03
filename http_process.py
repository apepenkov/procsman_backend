from http.server import BaseHTTPRequestHandler, HTTPServer
import sys
import threading
import os
import logging
import queue
from logging import NullHandler
import hashlib
import urllib.parse


logging.basicConfig(level=logging.ERROR + 1)
# app = Flask(__name__)
# for handler in logging.root.handlers[:]:
#     logging.root.removeHandler(handler)
# app.logger.addHandler(NullHandler())
# app.logger.setLevel(logging.ERROR + 1)
#
log = logging.getLogger("werkzeug")
log.setLevel(logging.ERROR)
for handler in log.handlers[:]:
    log.removeHandler(handler)
log.addHandler(NullHandler())

stdin_queue = queue.Queue()


def hashes(n=1000000):
    curr = b"0"
    for _ in range(n):
        curr = hashlib.sha256(curr).digest()

    return curr.hex()


def stdin_reader(q):
    for line in sys.stdin:
        q.put(line)


def load_ram(
    n_bytes=1024 * 1024 * 1024,
    remove_after=10,  # seconds
):
    """
    Load RAM with n_bytes of data, then remove it after remove_after seconds
    """
    data = b"\0" * n_bytes

    def clean():
        nonlocal data
        data = None

    threading.Timer(remove_after, lambda: clean).start()


class RequestHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/stop":
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"Stopping server\n")
            os._exit(0)  # Use os._exit to immediately terminate the process
        elif self.path == "/crash":
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"Crashing server\n")
            os._exit(1)
        elif self.path == "/stdin":
            try:
                data = stdin_queue.get_nowait()
            except queue.Empty:
                data = "stdin buffer is empty\n"
            self.send_response(200)
            self.end_headers()
            self.wfile.write(data.encode())
        elif self.path.startswith("/load_cpu"):
            # if param with number of hashes is passed, use it, otherwise use default
            query = urllib.parse.urlparse(self.path).query
            n = 1000000
            if query:
                if "n" in urllib.parse.parse_qs(query):
                    n_str = urllib.parse.parse_qs(query)["n"][0]
                    if n_str.isdigit():
                        n = int(n_str)
            r = hashes(n)
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"CPU loaded\n" + r.encode() + b"\n")
        elif self.path.startswith("/load_ram"):
            n_bytes = 1024 * 1024 * 1024
            seconds = 10
            query = urllib.parse.urlparse(self.path).query
            if query:
                if "n_bytes" in urllib.parse.parse_qs(query):
                    n_bytes_str = urllib.parse.parse_qs(query)["n_bytes"][0]
                    if n_bytes_str.isdigit():
                        n_bytes = int(n_bytes_str)
                if "seconds" in urllib.parse.parse_qs(query):
                    seconds_str = urllib.parse.parse_qs(query)["seconds"][0]
                    if seconds_str.isdigit():
                        seconds = int(seconds_str)
            load_ram(n_bytes, seconds)
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"RAM loaded\n")
        else:
            self.send_response(404)
            self.end_headers()

    def do_POST(self):
        content_length = int(self.headers["Content-Length"])
        post_data = self.rfile.read(content_length)

        if self.path == "/stdout":
            print(post_data.decode())
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"Printed to stdout\n")
        elif self.path == "/stderr":
            print(post_data.decode(), file=sys.stderr)
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"Printed to stderr\n")
        elif self.path == "/env":
            key = post_data.decode()
            value = os.environ.get(key, "Environment variable not found")
            self.send_response(200)
            self.end_headers()
            self.wfile.write(value.encode())
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        return


def run(server_class=HTTPServer, handler_class=RequestHandler, port=5431):
    server_address = ("", port)
    threading.Thread(target=stdin_reader, args=(stdin_queue,), daemon=True).start()
    httpd = server_class(server_address, handler_class)
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        httpd.server_close()


if __name__ == "__main__":
    run()
