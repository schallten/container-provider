import http.server
import socketserver
import os

PORT = 8000

class MyHandler(http.server.SimpleHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-type", "text/html")
        self.end_headers()
        
        html = f"""
        <html>
        <head><title>Tunnel Test</title></head>
        <body style="font-family: sans-serif; text-align: center; padding: 50px;">
            <h1 style="color: #238636;">🚀 Tunnel Works!</h1>
            <p>Environment ID: <strong>{os.getenv('HOSTNAME', 'unknown')}</strong></p>
            <p>Server Port: <strong>{PORT}</strong></p>
            <hr>
            <p style="color: #666;">This page is served from inside your TempDev container.</p>
        </body>
        </html>
        """
        self.wfile.write(html.encode())

print(f"Serving at http://localhost:{PORT}")
with socketserver.TCPServer(("", PORT), MyHandler) as httpd:
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        print("\nServer stopped.")
