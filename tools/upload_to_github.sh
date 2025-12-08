#!/usr/bin/env bash
# Simple-Trader/tools/upload_to_github.sh
#
# Upload the current project to GitHub:
# - Initialize a Git repo (if missing)
# - Create a minimal .gitignore (if missing)
# - Create an initial commit if there are no commits yet
# - Optionally create a GitHub repo using the `gh` cli and push
# - Optionally add a remote URL and push
#
# Usage:
#   # Dry run + show help
#   ./tools/upload_to_github.sh --help
#
#   # Create repo on GitHub and push (requires authenticated `gh` CLI)
#   ./tools/upload_to_github.sh --repo myusername/simple-trader --public --push --yes
#
#   # Add an existing remote and push
#   ./tools/upload_to_github.sh --remote git@github.com:myuser/simple-trader.git --push --yes
#
# Notes:
# - If you want automation without prompts, set --yes.
# - If the local repo has no commits and you use --yes, it will commit with "Initial commit".
# - If you prefer to control commits, don't pass --yes; the script will print instructions.
#
# Security & Best practices:
# - Ensure you don't push secrets; this script creates a recommended .gitignore (if missing)
# - If you have a `.env` file with secrets, keep it out of version control. Use `.env.example`.
#
# Auth:
# - If you prefer the `gh` CLI (recommended) to create a remote, make sure it's installed and authenticated:
#     gh auth login
# - Otherwise, pass --remote with the remote URL (SSH or HTTPS) and the script will add it (it won't create a GitHub repo).

set -euo pipefail

# Default values
REPO_NAME=""
REMOTE_URL=""
BRANCH="main"
PUBLIC=true
PUSH=false
YES=false
AUTO_COMMIT=false
FORCE=false
QUIET=false

# Helpers
print() { [ "${QUIET}" = "true" ] || echo -e "$@"; }
error() { echo "ERROR: $@" >&2; }
usage() {
  cat <<EOF
Upload-to-GitHub helper script - Simple-Trader/tools/upload_to_github.sh

Synopsis:
  ./tools/upload_to_github.sh [OPTIONS]

Options:
  --repo <owner/repo | repo>   Create a new GitHub repository via `gh` (owner optional).
  --remote <url>               Add an existing remote and push there (SSH or HTTPS).
  --public                     Mark repository as public (default).
  --private                    Mark repository as private.
  --branch <branch>            Branch to push (default: main).
  --push                       Push to the remote after creation.
  --yes, -y                    Non-interactive: auto-commit and auto-accept prompts.
  --auto-commit                Auto-stage & commit pending changes if present.
  --force                      Force overwrite an existing remote URL (if present).
  --quiet                      Reduce output verbosity.
  --help                       Show this help and exit.

Examples:
  # Create a new public repo using the gh CLI and push the current repo (requires gh)
  ./tools/upload_to_github.sh --repo "myuser/simple-trader" --public --push --yes

  # Add remote URL and push (use if repo already exists on GitHub)
  ./tools/upload_to_github.sh --remote "git@github.com:myuser/simple-trader.git" --push --yes

Notes:
  - If the repo has no commits, the script will create one ("Initial commit") if --yes specified.
  - If `gh` is not installed but --repo was specified, the script will instruct you how to create the repo manually.
EOF
  exit 0
}

# Parse args
while [[ $# -gt 0 ]]; do
  key=$1
  case "$key" in
    --repo)
      REPO_NAME="$2"; shift 2;;
    --remote|-u)
      REMOTE_URL="$2"; shift 2;;
    --public)
      PUBLIC=true; shift;;
    --private)
      PUBLIC=false; shift;;
    --branch|-b)
      BRANCH="$2"; shift 2;;
    --push)
      PUSH=true; shift;;
    --yes|-y)
      YES=true; shift;;
    --auto-commit)
      AUTO_COMMIT=true; shift;;
    --force)
      FORCE=true; shift;;
    --quiet)
      QUIET=true; shift;;
    --help|-h)
      usage;;
    *)
      echo "Unknown option: $1"; usage;;
  esac
done

# Check commands
command -v git >/dev/null 2>&1 || { error "git is required but not installed. Aborting."; exit 1; }
GH_PRESENT=false
if command -v gh >/dev/null 2>&1; then
  GH_PRESENT=true
fi

# Ensure we are in the project directory (script expects to be run from the repo root)
# We'll check for presence of `main.py` or `README.md` as a heuristic.
if [[ ! -f "main.py" && ! -f "README.md" ]]; then
  print "Warning: I don't see main.py or README.md in the current directory."
  print "Make sure you execute this script from the project root (Simple-Trader)."
fi

# Create a safe .gitignore if missing (minimal Python/generic example)
create_gitignore() {
  if [[ -f ".gitignore" ]]; then
    print ".gitignore already present - leaving it unchanged."
    return
  fi
  print "Creating a recommended .gitignore..."
  cat > .gitignore <<'EOF'
# Basic Python ignores and typical local artifacts
__pycache__/
*.py[cod]
*.so
*.egg
*.egg-info/
dist/
build/
pip-wheel-metadata/
.env
.env.*
*.db
*.sqlite
.simple_trader.db
.venv/
venv/
venv.bak/
.idea/
.vscode/
*.log
.DS_Store
*.pyc
.coverage
htmlcov/
.coverage.*
.pytest_cache/
EOF
  print ".gitignore created."
}

# Initialize repo if missing
ensure_git_repo() {
  if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    print "Git repository detected."
  else
    print "No git repo detected. Initializing git..."
    git init
    print "Git repository initialized."
  fi
}

# If no commits yet: create an initial commit
ensure_initial_commit() {
  if git rev-parse --verify HEAD >/dev/null 2>&1; then
    print "Repository already has commits."
    return
  fi

  # If there are changes that are not staged/committed, handle them
  CHANGES=$(git status --porcelain)
  if [[ -z "$CHANGES" ]]; then
    # nothing to commit
    print "No changes to commit (empty working tree). Creating an empty initial commit."
    git commit --allow-empty -m "Initial commit"
    return
  fi

  if [[ "$YES" = "true" || "$AUTO_COMMIT" = "true" ]]; then
    print "Staging all files and committing as 'Initial commit'..."
    git add -A
    git commit -m "Initial commit"
    return
  fi

  # Not auto-committing: prompt the user
  print "There are uncommitted changes:\n"
  git status --porcelain
  echo ""
  read -p "Do you want to stage and commit everything as 'Initial commit'? (y/n) " -r CONFIRM
  if [[ "$CONFIRM" =~ ^[Yy]$ ]]; then
    git add -A
    git commit -m "Initial commit"
    return
  fi

  error "Aborting: no initial commit made. Re-run with --yes or commit manually."
  exit 1
}

# Add / set remote origin
ensure_remote() {
  local url="$1"
  if [[ -z "$url" ]]; then
    error "No remote URL provided."
    exit 1
  fi
  if git remote get-url origin >/dev/null 2>&1; then
    existing=$(git remote get-url origin)
    if [[ "$existing" == "$url" ]]; then
      print "Remote origin already set to: $url"
      return
    fi
    if [[ "$FORCE" = "true" ]]; then
      git remote set-url origin "$url"
      print "Remote origin overwritten to $url"
      return
    else
      print "Remote origin already exists: $existing"
      error "Re-run with --force to overwrite or remove origin manually."
      exit 1
    fi
  fi
  git remote add origin "$url"
  print "Remote origin set to: $url"
}

# Create GitHub repo via gh CLI
create_github_repo() {
  local repo="$1"
  if [[ -z "$repo" ]]; then
    error "No repo name specified for creation."
    exit 1
  fi
  if [[ "$GH_PRESENT" != "true" ]]; then
    error "gh CLI is not installed - cannot create repository automatically."
    echo "Install 'gh' and run 'gh auth login' to authenticate, then re-run this script."
    exit 1
  fi

  # Build options
  if [[ "$PUBLIC" = "true" ]]; then
    VIS_FLAG="--public"
  else
    VIS_FLAG="--private"
  fi

  # If repo contains '/', use as 'owner/repo' else create under current user/org (gh defaults)
  print "Creating repository '$repo' via gh..."
  if gh repo view "$repo" >/dev/null 2>&1; then
    print "Repository '$repo' already exists on GitHub."
  else
    # Create repo: attach local source as remote, and create it
    # Use --confirm to avoid interactive prompts
    if gh repo create "$repo" $VIS_FLAG --source="." --remote=origin --push --confirm >/dev/null 2>&1; then
      print "Created GitHub repository and pushed current code to remote."
    else
      # Retry without --push if it fails (different gh versions)
      if gh repo create "$repo" $VIS_FLAG --source="." --remote=origin --confirm; then
        print "GitHub repo created, but could not push automatically. You can run 'git push -u origin ${BRANCH}' yourself."
      else
        error "Failed to create repo via gh. Please check your authentication and try again."
        exit 1
      fi
    fi
  fi
}

# Push to remote
do_push() {
  local branch="$1"
  # Ensure current branch name
  if git rev-parse --abbrev-ref HEAD >/dev/null 2>&1; then
    # We attempt to switch/create specified branch
    git branch --show-current >/dev/null 2>&1 && CURRENT_BRANCH=$(git branch --show-current) || CURRENT_BRANCH=""
  else
    CURRENT_BRANCH=""
  fi

  # If current branch is not target, try renaming/creating
  if [[ "$CURRENT_BRANCH" != "$branch" ]]; then
    if git rev-parse --verify "$branch" >/dev/null 2>&1; then
      git checkout "$branch"
    else
      git checkout -b "$branch"
    fi
  fi

  # push
  print "Pushing to origin ${branch}..."
  if git push -u origin "$branch"; then
    print "Push successful."
  else
    error "Failed to push. Please check your remote and permissions (SSH key or token)."
    exit 1
  fi
}

############################################
# Execution starts here
############################################

# Create .gitignore if missing
create_gitignore

# Ensure git and repo state
ensure_git_repo

# Create initial commit if needed
ensure_initial_commit

# If repo name provided: create repo via gh (preferred)
if [[ -n "$REPO_NAME" ]]; then
  if [[ "$GH_PRESENT" = "true" ]]; then
    create_github_repo "$REPO_NAME"
  else
    print "gh CLI not found. You can use 'gh repo create' to create a repo automatically."
    print "Alternatively run these commands to create a remote manually:"
    print "  # Create repo on GitHub via web UI or CLI, then:"
    print "  git remote add origin git@github.com:YOUR_USER/${REPO_NAME##*/}.git"
  fi
fi

# If a remote was provided explicitly, set it
if [[ -n "$REMOTE_URL" ]]; then
  ensure_remote "$REMOTE_URL"
fi

# If user didn't provide a remote and gh was used successfully, OR origin exists, we can push
if [[ "$PUSH" = "true" ]]; then
  # If no origin remote exists, try to infer from create via gh or prompt user to set remote.
  if ! git remote get-url origin >/dev/null 2>&1; then
    if [[ -n "$REPO_NAME" && "$GH_PRESENT" = "true" ]]; then
      # Should have been created and pushed by gh command; in some cases gh doesn't push; use repo name to construct remote
      origin_guess="git@github.com:${REPO_NAME}.git"
      print "Origin not found; attempting to add origin ${origin_guess} and push."
      git remote add origin "${origin_guess}"
    else
      error "No remote 'origin' is configured. Provide --remote or create a repo via 'gh' and re-run."
      exit 1
    fi
  fi
  do_push "$BRANCH"
else
  print "Push skipped (--push not provided)."
  if git remote get-url origin >/dev/null 2>&1; then
    ORIG_URL=$(git remote get-url origin)
    print "Remote origin is set: ${ORIG_URL}"
  else
    print "No remote 'origin' configured yet."
    print "If you want to push to GitHub now, re-run with --remote <url> or --repo <owner/repo> --push."
  fi
fi

print "Done. If you see authentication errors while pushing, make sure you have SSH keys or a GitHub token configured locally."
print ""
print "Next steps (examples):"
print "  # See current remote"
print "  git remote -v"
print ""
print "  # If you created the repo yourself, set branch protections or add a license"
print "  # Add a license (MIT recommended) by creating LICENSE in the repo and committing it"
print ""
print "  # Confirm push and verify on GitHub: https://github.com/your_user_or_org/your_repo"
