# Windows tray viewer for Blink and Tapo cameras

A compact Windows tray viewer for Blink and Tapo cameras. It combines a Gio flyout panel with a loopback web UI, live H.264/AAC playback, in-app clip playback, and pinned always-on-top camera windows.

## Features

- Tray flyout with camera grid, live view, snapshots, arm/disarm, and Sync Module status.
- Blink cloud cameras plus Tapo cameras over local RTSP with a Camera Account.
- Pin cameras into frameless always-on-top windows with mute, wheel zoom, drag pan and resize; pinned cameras reopen on launch.
- Pin the tray open, or drag its header to move it; the position is remembered.
- Per-camera motion toggle and Device Settings pages (motion, video, audio, privacy, zones) for Blink cameras.
- Sync Module page with firmware, Wi-Fi signal and storage used.
- Clips play inside the app with pause, seek and replay, from Sync Module storage or the Blink cloud.
- Optional local API and web UI on `http://127.0.0.1:7481/`.
- DPAPI-protected sessions and settings stored per user.

## Requirements

- Windows 10 or 11.
- Go 1.26.2 or newer to build.
- `ffmpeg` available in `PATH` for live audio/video decoding and fragmented MP4 packaging.
- A Blink account and/or a Tapo camera with a Camera Account. A Sync Module with local storage enabled is needed for Sync Module clips.

## Build

```powershell
go build -ldflags "-s -w -H=windowsgui" -o bin\CameraView.exe .
```

The checked-in `rsrc_windows_amd64.syso` supplies the executable icon and version resource.

## Run

```powershell
bin\CameraView.exe
```

Use the tray icon to open the flyout. `bin\CameraView.exe -show` opens the panel on startup. Sign in from the login view.

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
