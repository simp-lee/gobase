# Vendor Libraries

This directory holds third-party front-end libraries used by GoBase.  
The files are **not** checked into version control (see `.gitignore`).  
Download them manually before running the application:

## Required Files

| File              | Version | Download URL                                                        |
| ----------------- | ------- | ------------------------------------------------------------------- |
| `htmx.min.js`    | 2.0.4   | https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js                 |
| `alpine.min.js`   | 3.14.8  | https://unpkg.com/alpinejs@3.14.8/dist/cdn.min.js                  |
| `tailwind.css`    | 4.x     | https://cdn.tailwindcss.com/4 (Play CDN – save response as file)    |

## Quick Download (PowerShell)

```powershell
Invoke-WebRequest -Uri "https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js"    -OutFile "web/static/vendor/htmx.min.js"
Invoke-WebRequest -Uri "https://unpkg.com/alpinejs@3.14.8/dist/cdn.min.js"     -OutFile "web/static/vendor/alpine.min.js"
Invoke-WebRequest -Uri "https://cdn.tailwindcss.com/4"                          -OutFile "web/static/vendor/tailwind.css"
```

## Quick Download (curl)

```bash
curl -Lo web/static/vendor/htmx.min.js  "https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js"
curl -Lo web/static/vendor/alpine.min.js "https://unpkg.com/alpinejs@3.14.8/dist/cdn.min.js"
curl -Lo web/static/vendor/tailwind.css  "https://cdn.tailwindcss.com/4"
```

> **Note:** Tailwind CSS Play CDN is a development convenience. For production,
> consider using the Tailwind CLI to generate an optimised build.
