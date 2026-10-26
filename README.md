# Moreno.BlinkView

A compact Windows tray viewer for Blink cameras. It combines a Gio flyout panel with a loopback web UI, live H.264/AAC playback, Sync Module clips, and pop-out camera windows for multi-camera viewing.

## Features

- Tray flyout with camera grid, live view, snapshots, arm/disarm, and sync status.
- Live video with audio. Audio is only played by the panel camera that is open.
- Sync Module local-storage clips plus Blink cloud clips when available.
- Optional local API and web UI on `http://127.0.0.1:7481/`.
- Multiple simultaneous live streams.
- Picture-in-Picture button and separate pop-out player windows, one per camera.
- DPAPI-protected Blink session and settings stored per user.
- Hidden console windows for the app and ffmpeg child processes.

## Requirements

- Windows 10 or 11.
- Go 1.26.2 or newer to build.
- `ffmpeg` available in `PATH` for live audio/video decoding and fragmented MP4 packaging.
- A Blink account. A Sync Module with local storage enabled is needed for Sync Module clips.

## Build

```powershell
go build -ldflags "-s -w -H=windowsgui" -o bin\BlinkView.exe .
```

The checked-in `rsrc_windows_amd64.syso` supplies the executable icon and version resource.

## Run

```powershell
bin\BlinkView.exe
```

Use the tray icon to open the flyout. `bin\BlinkView.exe -show` opens the panel on startup. Sign in from the login view.

When the local API is enabled, open `http://127.0.0.1:7481/` in a browser. The API only listens on the loopback interface by default. Set a password in Settings if other local processes should not be able to use it.

## Storage

- Settings and the encrypted Blink session: `%APPDATA%\BlinkView`.
- Downloaded Sync Module clips: `%LOCALAPPDATA%\BlinkView\clips`.
- ffmpeg is required at runtime and is started without console windows.

## Development

```powershell
go vet ./...
go test ./...
```

`dev/shots` contains the headless UI shot renderer used during development:

```powershell
go run . -shot bin\shots
```

The main application source stays in the repository root. Build output belongs in `bin/`.

## API

The web UI uses the same loopback API:

- `GET /api/state`
- `POST /api/arm`
- `POST /api/refresh`
- `POST /api/cameras/{id}/wake`
- `GET /api/cameras/{id}/snapshot`
- `GET /api/cameras/{id}/mjpeg`
- `GET /api/cameras/{id}/live.mp4`
- `GET /api/clips`
- `GET /api/clips/{id}/video`

Live and clip responses are streamed. Clip video may take several seconds on the first request while the Sync Module prepares the recording. Later requests use the local cache.

## License

MIT
