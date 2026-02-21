# Vendor Libraries

This directory holds third-party front-end libraries used by GoBase.  
The files are **not** checked into version control (see `.gitignore`).  
Download them manually before running the application:

## Required Files

| File              | Version | Download URL                                                        |
| ----------------- | ------- | ------------------------------------------------------------------- |
| `htmx.min.js`    | 2.0.4   | https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js                 |
| `alpine.min.js`   | 3.14.8  | https://unpkg.com/alpinejs@3.14.8/dist/cdn.min.js                  |
| `tailwind.js`     | 4.2.0   | https://unpkg.com/@tailwindcss/browser@4                            |

## Quick Download (PowerShell)

```powershell
Invoke-WebRequest -Uri "https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js"    -OutFile "web/static/vendor/htmx.min.js"
Invoke-WebRequest -Uri "https://unpkg.com/alpinejs@3.14.8/dist/cdn.min.js"     -OutFile "web/static/vendor/alpine.min.js"
Invoke-WebRequest -Uri "https://unpkg.com/@tailwindcss/browser@4"               -OutFile "web/static/vendor/tailwind.js"
```

## Quick Download (curl)

```bash
curl -Lo web/static/vendor/htmx.min.js  "https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js"
curl -Lo web/static/vendor/alpine.min.js "https://unpkg.com/alpinejs@3.14.8/dist/cdn.min.js"
curl -Lo web/static/vendor/tailwind.js   "https://unpkg.com/@tailwindcss/browser@4"
```

> **Note:** This repository vendors the Tailwind CSS v4 browser JIT compiler (`@tailwindcss/browser`).
> It runs in the browser and generates only the CSS your templates actually use — no build step required.
> For high-traffic production sites, consider using the Tailwind CLI for pre-built CSS.
