"""Build the static frontend for GitHub Pages (no third-party dependencies)."""
import json
import os
from pathlib import Path
import shutil
from urllib.parse import urlsplit

root = Path(__file__).resolve().parents[1]
api_base = os.environ.get("API_BASE_URL", "").rstrip("/")
url = urlsplit(api_base)
if (url.scheme != "https" or not url.hostname or url.username or url.password
        or url.path or url.query or url.fragment):
    raise SystemExit("Set API_BASE_URL to the backend HTTPS origin, without a path.")

output = root / "artifacts" / "pages"
if output.exists():
    shutil.rmtree(output)
shutil.copytree(root / "internal" / "httpapi" / "web", output)
(output / "config.js").write_text(
    "window.MC_CONFIG = " + json.dumps({"apiBaseUrl": api_base}) + ";\n",
    encoding="utf-8",
)
(output / ".nojekyll").touch()
print(f"Frontend built in {output}")
