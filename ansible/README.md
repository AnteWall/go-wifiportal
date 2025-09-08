# Ansible Firewall Setup

This directory contains a simple Ansible configuration to set up UFW and iptables access without sudo password for a specific user on remote hosts.

## Files

- `ansible.cfg` - Basic Ansible configuration
- `hosts.yml` - Inventory file with host definitions
- `setup-ufw.yml` - Playbook to configure firewall sudo access

## Usage

1. **Update the inventory**: Edit `hosts.yml` and replace the example host details with your actual host information:
   ```yaml
   wifiportal-host:
     ansible_host: YOUR_HOST_IP
     ansible_user: YOUR_USERNAME
     # ansible_ssh_private_key_file: ~/.ssh/your_private_key
   ```

2. **Run the setup playbook**:
   ```bash
   cd ansible
   ansible-playbook setup-ufw.yml --ask-become-pass
   ```

3. **Test firewall access**: After running the playbook, the specified user should be able to run firewall commands without entering a sudo password:
   ```bash
   ssh your_user@your_host
   sudo ufw status              # Should work without password prompt
   sudo iptables -L -n          # Should work without password prompt
   sudo iptables-legacy -L -n   # Should work without password prompt (if legacy exists)
   ```

## What it does

The playbook will:
1. Install UFW, iptables, and iptables-persistent packages
2. Check if iptables-legacy tools exist on the system
3. Create a sudoers file that allows the specified user to run firewall commands without a password:
   - `/usr/sbin/ufw`
   - `/usr/sbin/iptables` and related tools
   - `/usr/sbin/iptables-legacy` and related tools (if they exist)
   - IPv6 variants of all tools
4. Test that all firewall tools can be accessed without sudo password

## Supported Commands

After running this playbook, the user can run these commands without sudo password:
- `sudo ufw` (all UFW commands)
- `sudo iptables`, `sudo iptables-save`, `sudo iptables-restore`
- `sudo ip6tables`, `sudo ip6tables-save`, `sudo ip6tables-restore`
- `sudo iptables-legacy`, `sudo iptables-legacy-save`, `sudo iptables-legacy-restore` (if available)
- `sudo ip6tables-legacy`, `sudo ip6tables-legacy-save`, `sudo ip6tables-legacy-restore` (if available)

## Requirements

- Ansible installed on the control machine
- SSH access to the target host
- The target user must have sudo privileges (for the initial setup)
- Python 3 on the target host
