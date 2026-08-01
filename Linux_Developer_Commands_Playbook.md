# Linux Developer Commands Playbook

A practical handbook for daily developer work on Linux.

## How To Learn Fast

- Practice 10 commands every day for 15 minutes.
- Learn by job-to-be-done, not by alphabet.
- Use history and aliases to reduce typing.
- Keep this file open in split view while working.

## 1) Basic Navigation And File Operations

### Where am I and what is here

```bash
pwd              # Print current working directory
ls               # List files/folders
ls -la           # List all files, including hidden, with details
ls -lh           # List with human-readable file sizes
```

### Move around

```bash
cd /path/to/dir  # Change to a specific directory
cd ~             # Go to home directory
cd -             # Go back to previous directory
```

### Create, copy, move, delete

```bash
mkdir project                         # Create one directory
mkdir -p app/{cmd,internal,configs}  # Create nested directories in one command
touch notes.txt                       # Create empty file (or update timestamp)
cp source.txt backup.txt              # Copy file
cp -r src/ src_backup/                # Copy directory recursively
mv old.txt new.txt                    # Rename or move file
rm file.txt                           # Remove file
rm -r folder/                         # Remove directory recursively
rm -rf build/                         # Force remove directory recursively (dangerous)
```

### View file content

```bash
cat file.txt         # Print full file content
less file.txt        # Open file with scroll/pager view
head -n 20 file.txt  # Show first 20 lines
tail -n 20 file.txt  # Show last 20 lines
tail -f app.log      # Follow log file in real time
```

## 2) Search And Text Processing

### Find files

```bash
find . -name "*.go"        # Find files by name pattern
find . -type f -size +10M  # Find files larger than 10 MB
```

### Fast text search (developer favorite)

```bash
rg "Open\(" .         # Search text pattern in current tree
rg "TODO|FIXME" -n   # Search TODO/FIXME and show line numbers
rg --files           # List files quickly
```

### Grep, sed, awk

```bash
grep -R "journal_mode" .           # Recursively search text in files
grep -Rin "error" logs/            # Recursive + case-insensitive + line numbers
sed 's/old/new/g' file.txt         # Replace all old with new in output stream
awk '{print $1}' data.txt          # Print first column from each line
cut -d',' -f1 users.csv            # Extract first CSV column
sort names.txt                     # Sort lines alphabetically
uniq names.txt                     # Remove adjacent duplicate lines
wc -l file.txt                     # Count lines in file
```

## 3) Permissions, Ownership, And Links

```bash
chmod 644 file.txt                          # Owner read/write, others read
chmod +x script.sh                          # Add executable permission
chmod -R 755 scripts/                       # Set recursive execute/read permissions
chown user:user file.txt                    # Change owner and group of file
chown -R user:user app/                     # Change owner and group recursively
ln -s /opt/app/config.yaml ./config.yaml    # Create symbolic link
```

Quick memory:

- 4 = read
- 2 = write
- 1 = execute
- 755 means owner full, group and others read+execute.

## 4) Processes, System, And Service Management

### Process control

```bash
ps aux                 # Show all running processes
ps -ef | rg nginx      # Find nginx process in process list
top                    # Live process/resource monitor
htop                   # Interactive process monitor (friendlier top)
pgrep -a python        # Find process IDs and commands matching python
kill 1234              # Send default TERM signal to PID 1234
kill -9 1234           # Force kill process with SIGKILL
pkill -f "my-app"      # Kill processes matching full command pattern
```

### Systemd services

```bash
systemctl status nginx   # Show current service status
systemctl start nginx    # Start service now
systemctl stop nginx     # Stop service now
systemctl restart nginx  # Restart service now
systemctl enable nginx   # Start service automatically at boot
systemctl disable nginx  # Disable service auto-start at boot
systemctl daemon-reload  # Reload systemd unit files after changes
```

### Logs and boot info

```bash
journalctl -u nginx                 # Show logs for nginx service
journalctl -u nginx -f              # Follow nginx logs live
journalctl -xe                      # Show recent important system logs
journalctl --since "1 hour ago"     # Show logs from the last hour
dmesg | tail                        # Show latest kernel messages
```

### System information

```bash
uname -a            # Show kernel and system architecture info
hostnamectl         # Show hostname and OS metadata
cat /etc/os-release # Show Linux distribution details
uptime              # Show system uptime and load average
free -h             # Show memory usage in human-readable format
df -h               # Show disk free space by filesystem
du -sh *            # Show size of each item in current directory
lsblk               # List block devices and partitions
```

## 5) Networking Commands For Developers

```bash
ip a                                   # Show network interfaces and IP addresses
ip r                                   # Show routing table
ss -tulnp                              # Show listening TCP/UDP ports with process names
ping -c 4 8.8.8.8                      # Send 4 ICMP packets to test connectivity
curl -I https://example.com            # Fetch response headers only
curl -s https://api.github.com | jq .  # Fetch API JSON quietly and pretty-print
wget https://example.com/file.tar.gz   # Download file from URL
nslookup example.com                   # Query DNS records
dig example.com                        # Detailed DNS lookup
traceroute example.com                 # Trace packet path to destination host
```

Port check examples:

```bash
ss -ltnp | rg 8080  # Check if port 8080 is listening
lsof -i :8080       # Show process using port 8080
```

## 6) Packaging And Archives

### Tar, gzip, zip

```bash
tar -cvf app.tar app/       # Create tar archive
tar -xvf app.tar            # Extract tar archive
tar -czvf app.tar.gz app/   # Create compressed tar.gz archive
tar -xzvf app.tar.gz        # Extract compressed tar.gz archive
zip -r app.zip app/         # Create recursive zip archive
unzip app.zip               # Extract zip archive
```

### Build Debian package

```bash
dpkg-deb --build mypkg  # Build .deb package from directory structure
```

### Inspect/install Debian package

```bash
dpkg -I mypkg.deb      # Show package metadata (name, version, deps)
dpkg -c mypkg.deb      # List files inside .deb package
sudo dpkg -i mypkg.deb # Install .deb package
sudo apt -f install    # Fix and install missing dependencies
```

### Build RPM package

```bash
rpmbuild -ba SPECS/mypkg.spec  # Build source and binary RPM from spec file
```

### Inspect/install RPM package

```bash
rpm -qpi mypkg.rpm       # Show RPM package information
rpm -qpl mypkg.rpm       # List files included in RPM
sudo rpm -ivh mypkg.rpm  # Install RPM package (fresh install)
sudo rpm -Uvh mypkg.rpm  # Upgrade or install RPM package
```

## 7) Linux Package Managers (Install Software)

### Debian/Ubuntu (apt)

```bash
sudo apt update                     # Refresh package index
sudo apt upgrade -y                # Upgrade installed packages
sudo apt install -y git curl vim   # Install packages
sudo apt remove package_name       # Remove package (keep configs)
sudo apt purge package_name        # Remove package and config files
sudo apt autoremove -y             # Remove unused dependencies
apt search package_name            # Search packages by name
apt show package_name              # Show package details
```

### RHEL/CentOS/Fedora (dnf/yum)

```bash
sudo dnf check-update              # Check available package updates
sudo dnf upgrade -y                # Upgrade installed packages
sudo dnf install -y git curl vim   # Install packages
sudo dnf remove package_name       # Remove package
dnf search package_name            # Search package repository
dnf info package_name              # Show package metadata
```

Legacy systems:

```bash
sudo yum install -y git curl vim  # Install packages on older yum-based systems
```

### Arch Linux (pacman)

```bash
sudo pacman -Syu               # Sync repos and full system upgrade
sudo pacman -S git curl vim    # Install packages
sudo pacman -R package_name    # Remove package
pacman -Ss package_name        # Search package in repos
```

### openSUSE (zypper)

```bash
sudo zypper refresh               # Refresh repository metadata
sudo zypper update                # Update installed packages
sudo zypper install git curl vim  # Install packages
sudo zypper remove package_name   # Remove package
zypper search package_name        # Search for package
```

### Snap and Flatpak

```bash
sudo snap install code --classic         # Install VS Code from snap
snap list                                # List installed snap packages
sudo snap remove code                    # Remove snap package

flatpak install flathub org.mozilla.firefox  # Install Firefox from Flatpak
flatpak list                                 # List installed Flatpak apps
flatpak uninstall org.mozilla.firefox        # Uninstall Flatpak app
```

## 8) Developer Workflow Commands

### Git essentials

```bash
git status                                # Show changed/staged files
git add .                                 # Stage all current changes
git commit -m "message"                   # Commit staged changes
git log --oneline --graph --decorate -20 # Compact graph view of last 20 commits
git diff                                  # Show unstaged code differences
git restore file.txt                      # Discard unstaged changes in file
git checkout -b feature/new-work          # Create and switch to new branch
git pull --rebase                         # Pull remote changes with rebase
git push -u origin feature/new-work       # Push branch and set upstream
```

### SSH and remote copy

```bash
ssh user@server                      # Open SSH session to remote server
scp file.txt user@server:/tmp/       # Copy local file to remote path
scp -r app/ user@server:/opt/        # Copy local directory recursively
rsync -avz app/ user@server:/opt/app/ # Efficient sync/copy with compression
```

### Environment variables

```bash
env                   # Print all environment variables
printenv | rg HOME    # Filter env vars containing HOME
export APP_ENV=dev    # Set environment variable in current shell
echo $APP_ENV         # Print variable value
```

### Command history and aliases

```bash
history                # Show command history
history | tail -n 50   # Show last 50 history entries
alias ll='ls -la'      # Create shortcut alias for detailed list command
alias gs='git status'  # Create shortcut alias for git status
```

## 9) Vim Editor Commands

## Modes

- Normal mode: navigation and commands
- Insert mode: typing text
- Visual mode: selection
- Command mode: save, quit, search, replace

### Open and quit

```bash
vim file.txt  # Open file in Vim editor
```

Inside Vim:

- i: enter insert mode
- Esc: back to normal mode
- :w: save
- :q: quit
- :wq: save and quit
- :q!: quit without saving

### Navigation

- h j k l: left down up right
- 0: line start
- $: line end
- gg: top of file
- G: end of file
- :42: go to line 42

### Edit basics

- x: delete char
- dd: delete line
- yy: copy line
- p: paste below
- u: undo
- Ctrl+r: redo

### Search and replace

- /text: search forward
- n: next match
- N: previous match
- :%s/old/new/g: replace all
- :%s/old/new/gc: replace all with confirmation

### Useful Vim settings for beginners

Add to ~/.vimrc:

```vim
set number                " Show absolute line numbers
set relativenumber        " Show relative line numbers for fast jumps
set tabstop=4             " Display tab as 4 spaces wide
set shiftwidth=4          " Indent/unindent by 4 spaces
set expandtab             " Convert tab key to spaces
set hlsearch              " Highlight all search matches
set incsearch             " Show match while typing search
set ignorecase            " Ignore case in searches by default
set smartcase             " Override ignorecase when search has uppercase
set clipboard=unnamedplus " Use system clipboard
```

## 10) Useful One-Liners For Daily Work

```bash
# Top 20 largest files
find . -type f -exec du -h {} + | sort -hr | head -n 20

# Show open ports
ss -tulnp

# Follow logs with filtering
journalctl -f | rg "error|fail|panic"

# Check disk usage per folder
du -h --max-depth=1 | sort -hr

# Kill process by port
kill -9 $(lsof -t -i:8080)
```

## 11) Safe Command Habits

- Run read-only commands first: ls, cat, grep, rg.
- Before rm -rf, run ls on the same path.
- Prefer mv to a backup folder instead of immediate delete.
- Use sudo only when needed.
- Check current directory with pwd before destructive actions.

## 12) 7-Day Practice Plan (Memorization Friendly)

Day 1:

- pwd, ls, cd, mkdir, touch, cp, mv, rm

Day 2:

- cat, less, head, tail, rg, find, grep

Day 3:

- chmod, chown, ln -s, df, du, free

Day 4:

- ps, top, kill, systemctl, journalctl

Day 5:

- ip, ss, ping, curl, wget, lsof

Day 6:

- apt/dnf/pacman/zypper basics, tar/zip commands

Day 7:

- vim basics, git basics, ssh/scp/rsync

---

If you want, this playbook can be split into separate files next:

- Linux_Basics.md
- Packaging_Commands.md
- Systemd_And_Logs.md
- Vim_Quickstart.md
