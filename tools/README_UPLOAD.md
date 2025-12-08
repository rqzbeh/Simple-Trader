Simple-Trader/tools/README_UPLOAD.md#L1-999
# Uploading Simple-Trader to GitHub

This document explains how to upload (push) the Simple-Trader repository to GitHub. There is a helper script in `tools/upload_to_github.sh` that automates the common steps (initialize repo, add a default `.gitignore`, create an initial commit, create a GitHub repo via `gh`, add a remote, and push). This README also contains manual steps and troubleshooting if you prefer to do it yourself.

> Important safety note: Do not store API keys, tokens, or other secrets in the repository. Always use `.env` (which must be excluded via `.gitignore`) and store secrets using your OS secret manager, GitHub secrets (CI), or a vault.

---

## Table of Contents

1. Overview
2. Requirements
3. Quick example (one-liners)
4. Using the `tools/upload_to_github.sh` script
5. Manual steps to create & push a repo
6. Create a GitHub repo with `gh` CLI
7. Create a GitHub repo via API (curl + PAT)
8. Troubleshooting
9. Post-upload checklist & recommended next steps
10. If you want me to push on your behalf

---

## 1) Overview

The helper script exists to make it easy to push the repository to GitHub by automating a few tasks:

- Set a minimally recommended `.gitignore` if missing.
- Initialize a `git` repository if not present.
- Make an initial commit (if missing).
- Optionally create a remote GitHub repository using the `gh` CLI.
- Optionally add a specific remote URL and push your code to it.

The script is not required; the manual commands are also provided if you prefer to use the web UI or other tooling.

---

## 2) Requirements

- Git must be installed and configured with your user name and email.
- The `gh` CLI (optional but recommended) if you want the script to create the remote repository and push automatically.
- For SSH uploads: Ensure you have your SSH key added to your GitHub account:
  - `ssh-keygen -t ed25519 -C "your-email@example.com"`
  - `eval "$(ssh-agent -s)"; ssh-add ~/.ssh/id_ed25519`
  - Add the public key to GitHub > Settings > SSH and GPG keys
- For HTTPS uploads: Use `git` with a credential helper or generate a GitHub PAT.

---

## 3) Quick example (one-liners)

- Using the helper script:
```Simple-Trader/tools/README_UPLOAD.md#L100-108
# Create a new public repo via gh, auto-commit and push
cd Simple-Trader
./tools/upload_to_github.sh --repo username/simple-trader --public --push --yes
```

- If you already created the remote repo on GitHub (or want to add a remote manually) and push:
```Simple-Trader/tools/README_UPLOAD.md#L110-116
cd Simple-Trader
git add -A
git commit -m "Initial commit"
git remote add origin git@github.com:username/simple-trader.git
git branch -M main
git push -u origin main
```

---

## 4) Using the `tools/upload_to_github.sh` script

The script supports the following options:

- `--repo <owner/repo>` — Create a new GitHub repository using the `gh` CLI. If the `owner` portion is omitted, your authenticated `gh` user is used.
- `--remote <git_remote_url>` — Add an existing remote instead of creating a new repository.
- `--public|--private` — Choose visibility for a new repository (public by default).
- `--branch <branch>` — Choose the branch to push (default: `main`).
- `--push` — Push to the remote after creation (if possible).
- `--yes` or `-y` — Run non-interactive: automatically commit and avoid confirmation prompts.
- `--auto-commit` — If there are changes and you prefer to automatically stage + commit them.
- `--force` — Replace an existing `origin` remote if present.
- `--quiet` — Reduce script output.

Example with a pre-existing remote:
```Simple-Trader/tools/README_UPLOAD.md#L140-146
# Create the initial commit and push to an existing repo
cd Simple-Trader
./tools/upload_to_github.sh --remote git@github.com:username/simple-trader.git --push --yes
```

Notes:
- If you provide `--repo` and `gh` is installed and authenticated, `gh` will attempt to create the repository and push.
- If `gh` is not installed, the script will guide you to add a remote manually.

---

## 5) Manual steps (recommended if you prefer to do everything by hand)

1. Go to the repository root:
```Simple-Trader/tools/README_UPLOAD.md#L170-173
cd Simple-Trader
```

2. Make sure `.gitignore` exists and excludes local secrets, db files, and the `.env` file (script will create one if missing):
```Simple-Trader/tools/README_UPLOAD.md#L175-182
# Example:
cat > .gitignore <<'EOF'
.env
.env.*
*.db
*.sqlite
.venv/
__pycache__/
*.pyc
EOF
```

3. Initialize git if needed and create initial commit:
```Simple-Trader/tools/README_UPLOAD.md#L186-193
git init
git add -A
git commit -m "Initial commit"
git branch -M main
```

4. Add remote and push:
- With SSH:
```Simple-Trader/tools/README_UPLOAD.md#L200-204
git remote add origin git@github.com:username/simple-trader.git
git push -u origin main
```

- With HTTPS:
```Simple-Trader/tools/README_UPLOAD.md#L208-212
git remote add origin https://github.com/username/simple-trader.git
# Push (you will be prompted for credentials or PAT)
git push -u origin main
```

---

## 6) Create a GitHub repository with `gh` CLI (recommended)

If you have the GitHub CLI set up (and authenticated via `gh auth login`), you can create a new repo and push in a single step. Example:
```Simple-Trader/tools/README_UPLOAD.md#L230-242
# Create a new public repo and push
cd Simple-Trader
gh auth login   # (only if not authenticated)
# This creates repo "myuser/simple-trader" and pushes your current code
./tools/upload_to_github.sh --repo myuser/simple-trader --public --push --yes
```

If `gh` fails to push automatically, simply add the remote (if missing) and push:
```Simple-Trader/tools/README_UPLOAD.md#L246-252
git remote add origin git@github.com:myuser/simple-trader.git
git push -u origin main
```

---

## 7) Create a GitHub repo via API (curl + PAT)

If you don't use `gh` CLI but have a GitHub Personal Access Token (PAT), you can create a repo with the API:

```Simple-Trader/tools/README_UPLOAD.md#L270-286
# Create a repo using your GitHub PAT (replace <PAT> and values)
curl -H "Authorization: token <PAT>" \
     -d '{"name":"simple-trader","private":true}' \
     https://api.github.com/user/repos

# After successful creation, add the remote and push:
git remote add origin git@github.com:username/simple-trader.git
git push -u origin main
```

Note: Ensure your PAT has `repo` scope. For an organization repo, you need `admin:org` or appropriate permissions.

---

## 8) Troubleshooting

1. `git push` fails with auth errors:
   - If using SSH: ensure `ssh-agent` has your key loaded and your key was added to GitHub.
     - Test with `ssh -T git@github.com`
   - If using HTTPS: generate a personal access token (classic) with `repo` scope, or use a credential manager.

2. Script reports "gh CLI not installed" when `--repo` was used:
   - Install `gh` from https://cli.github.com/
   - Run `gh auth login` to authenticate.

3. You accidentally committed a secret:
   - Remove it from future commits:
     ```Simple-Trader/tools/README_UPLOAD.md#L320-330
     git rm --cached .env
     echo ".env" >> .gitignore
     git commit -m "Remove .env and add to .gitignore"
     git push
     ```
   - If the secret was pushed to a remote, you must remove it from history using `git filter-repo` or `BFG Repo-Cleaner` and rotate the compromised secret. See GitHub docs for removing sensitive data.

4. There is no initial commit and you want to keep working on change history before pushing:
   - You can commit the code and push later; there is no required push step by the script unless you supply `--push`.

5. GitHub rate limits or two-factor auth problems:
   - Use `gh` and `gh auth login` or remote via SSH. PAT and 2FA are supported via `gh auth login`.

---

## 9) Post-upload checklist & recommended next steps

- On GitHub:
  - Add a LICENSE (e.g., MIT) if you want others to use or contribute.
  - Configure branch protection rules for `main` and enable required status checks.
  - Add the project secrets in the repository settings for CI or Actions.
  - Create a minimal GitHub Actions workflow for tests, linting, or CI if desired.
  - If using `gh` CLI, you can create an initial PR template or contribution guidelines.

- Locally:
  - Set up a development virtualenv and install `requirements.txt`.
  - Ensure `.env.example` is checked in while `.env` is ignored.

---

## 10) If you want me to push on your behalf

- I cannot access your GitHub account or push directly for you here without your GitHub credentials or an authorized `gh` session.
- If you want me to perform the push for you:
  - Provide the remote URL (or owner/repo for creation).
  - Confirm whether you prefer public or private visibility for the new repository.
  - Grant `gh`-CLI based authorization for me to use (if you trust the environment). Note: Sharing tokens or credentials in public chat is unsafe—prefer to run the script locally or use `gh` authentication.

---

If you'd like, I can:
- Add a LICENSE (e.g., MIT) and/or initial GitHub Actions workflow in this repo.
- Run the commit-only steps locally (create `.gitignore`, commit) and provide the `git remote` for you to push.
- Or walk you through the `gh` or `curl` steps interactively.

Tell me which option you prefer:
- "auto-create" (create and push using `gh` CLI),
- "push-to-existing" (I'll set remote and commit; you'll push),
- or "provide-instructions" (I'll give step-by-step commands for you).
