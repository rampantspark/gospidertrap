<p align="center">
  <a>
    <img src="https://skillicons.dev/icons?i=go,linux,apple,windows,docker" />
  </a>
</p>

This program is inspired by [spidertrap](https://github.com/adhdproject/spidertrap).

HTML Input (link replacement)
 - Provide an HTML file as input and the links with be replaced with randomly generated ones.

HTML Input (form submit action)
  - Provide an HTML file containing a form that submits a get request to endpoint and links will be procedurally generated on form submit.

Wordlist
  - All links can be picked from a wordlist instead of procedurally generated. 

## Installation

### From GitHub Releases
Download the latest binary for your platform from the [releases page](https://github.com/rampantspark/gospidertrap/releases).

### Using Docker
```bash
docker pull ghcr.io/rampantspark/gospidertrap:latest
docker run -p 8000:8000 ghcr.io/rampantspark/gospidertrap:latest
```

### From Source
```bash
git clone https://github.com/rampantspark/gospidertrap.git
cd gospidertrap
make build
./gospidertrap
```

## Usage

### Quick Start
Start the server with default settings (procedurally generated links):
```bash
./gospidertrap
```

The server will start on port 8000. Check the console output for the admin panel URL and login token.

### Common Examples

**Use a custom wordlist for links:**
```bash
./gospidertrap -w wordlist.txt
```

**Replace links in an HTML template:**
```bash
./gospidertrap -a template.html -w wordlist.txt
```

**Generate links on form submission:**
```bash
./gospidertrap -a form.html -e /submit -w wordlist.txt
```

**Custom port and rate limiting:**
```bash
./gospidertrap -p 3000 -rate-limit 20 -rate-burst 40
```

**Run behind a reverse proxy:**
```bash
./gospidertrap -https -trust-proxy
```

### Command-Line Options

| Flag | Description | Default |
|------|-------------|---------|
| `-p` | Port to run the server on | `8000` |
| `-a` | HTML file input, replace `<a href>` links | - |
| `-e` | Endpoint for form GET requests | - |
| `-w` | Wordlist file to use for links | - |
| `-d` | Data directory for persistence | `data` |
| `-db-path` | Path to SQLite database file | `data/stats.db` |
| `-use-files` | Use legacy file-based persistence instead of SQLite | `false` |
| `-rate-limit` | Rate limit: requests per second per IP | `10` |
| `-rate-burst` | Rate limit: burst size per IP | `20` |
| `-https` | Enable HTTPS mode (sets Secure flag on cookies) | `false` |
| `-trust-proxy` | Trust X-Forwarded-For and X-Real-IP headers | `false` |

### Admin Panel

After starting the server, the console will display:
- **Login URL**: One-time login link to access the admin panel
- **Admin URL**: Direct admin panel URL (requires authentication)

The admin panel provides:
- Real-time request statistics
- IP address tracking
- User agent analysis
- Request history
- Visual charts and graphs

### Docker Compose

For a complete setup with Traefik reverse proxy:
```bash
docker-compose -f docker-compose.traefik.yml up -d
```

Standard docker-compose:
```bash
docker-compose up -d
```
