# lanchat

A tiny chat server for when the internet is down but the college LAN still works.
One person (the host) runs a single program, shares a link, and everyone on the same
network opens it in their browser and chats. No app, no accounts, no internet.

Go standard library only. No outside packages, nothing to download at runtime.
The web page has no CDN or external files, so it works fully offline.

## Run it

You need Go 1.22 or newer on the host's laptop.

```
go run .
```

It prints a link like `http://10.1.4.23:8080/?k=x7m2qa`. Send that to people on the
same network. They open it, type a name, and chat.

Options:

```
-port 8080         port to use
-room "Hostel 3"   name shown at the top
-key abc123        set the key yourself (random if you skip it)
-no-key            no key, anyone on the network can join
-keep 500          how many messages to keep in memory
-max-clients 200   max people connected at once
```

Build a single file you can copy to another laptop:

```
go build -o lanchat .
GOOS=windows GOARCH=amd64 go build -o lanchat.exe .
GOOS=darwin GOARCH=arm64 go build -o lanchat-mac .
```

Run the tests: `go test -race ./...`

## How it works

- `main.go`      flags, finds the host's LAN addresses, prints the join link, clean shutdown
- `server.go`    the http handlers (page, send, live stream, info) and the key check
- `hub.go`       keeps the last N messages in memory and pushes new ones to everyone
- `limiter.go`   rate limit per IP so one person can't flood the room
- `validate.go`  cleans names and messages
- `web/index.html` the chat page (embedded into the program with go:embed)

Messages come to the browser with Server-Sent Events (one open connection, server pushes).
Sending is a normal POST. If a phone's wifi drops, the browser reconnects by itself and
sends the last message id it saw, so it gets what it missed.

## Things that can go wrong on a real college network

- **Wifi "client isolation"**: some access points stop devices from talking to each other.
  Then nobody can reach the host. Wired ethernet in labs is usually fine. Test it first.
- **Different VLANs/subnets**: hostel and department networks may be separated. Only people
  who can reach the host's IP can join.
- **Host firewall**: Windows will ask to allow the program through. Say yes for private networks.
- **Sharing the link with no internet**: WhatsApp won't work. Read it out, write it on a board,
  or show it on a projector. Keep the key short (it's 6 characters) for this reason.
- **If the LAN dies too**: the host can turn on a phone hotspot, connect the laptop to it,
  run the server, and others join that hotspot.

## Privacy and security (read this)

- It's plain http, not https. Anyone else on the network who is sniffing traffic can read
  the chat and see the key. Don't use it for anything sensitive.
- The key only keeps random people out. It is not strong security.
- Messages are only in memory. Stop the server (Ctrl+C) and everything is gone.
  The server never logs message text or the key.
- Names are not verified. Anyone can type any name.

## Ideas for later

- QR code for the join link (needs a small library)
- Multiple rooms
- Host controls: clear room, kick someone
- Optional save to SQLite
- Dockerfile + GitHub Actions to run tests on every push
