# Discordo &middot; [![discord](https://img.shields.io/discord/1297292231299956788?color=5865F2&logo=discord&logoColor=white)](https://discord.com/invite/VzF9UFn2aB) [![license](https://img.shields.io/github/license/ayn2op/discordo?logo=github)](https://github.com/ayn2op/discordo/blob/master/LICENSE)

Discordo is a lightweight, secure, and feature-rich Discord terminal client.

> [!NOTE]
> This is [stefanw1337](https://github.com/stefanw1337)'s fork, built on [BalajiLeninrajan/discordo](https://github.com/BalajiLeninrajan/discordo). It adds watching others' Go Live streams (video in [mpv](https://mpv.io), audio with the voice channel), a profile card, a mentions inbox, favorite channels, a message menu on right-click, sticker labels and more. Ready-to-run files for Windows, Linux, macOS and Android (Termux) are on the [releases page](https://github.com/stefanw1337/discordo/releases): download the one for your system, unpack it and run `discordo`.
>
> To watch streams you need [mpv](https://mpv.io). The Windows downloads include it in an `mpv` folder, where discordo finds it. Elsewhere, install mpv with your package manager, or set `stream.player` in the config to where it is. Join the voice channel, select the member marked LIVE and press `w`.

![Preview](.github/preview.png)
![Picker](.github/picker.png)

## Installation

> [!NOTE]
> The release files of this fork are not code-signed, so a browser or antivirus program may warn about them. See [About antivirus and browser warnings](.github/RELEASE_NOTE.md) for why, and how to check a file.

### Prebuilt binaries

You can download and install a [prebuilt binary here](https://nightly.link/ayn2op/discordo/workflows/ci/main) for Windows, macOS, or Linux.

### Package managers

- Arch Linux: `yay -S discordo-git`
- Gentoo (available on the guru repos as a live ebuild): `emerge net-im/discordo`
- FreeBSD: `pkg install discordo` or via the ports system `make -C /usr/ports/net-im/discordo install clean`
- Nix: Add `pkgs.discordo` to `environment.systemPackages` or `home.packages`
- Android (Termux): `pkg install discordo`

- Windows (Scoop):

```sh
scoop bucket add vvxrtues https://github.com/vvirtues/bucket
scoop install discordo
```

### Building from source

Requires [Go](https://go.dev/dl). Voice support requires a C compiler, and `libpulse` or `libasound` on Linux.

```sh
git clone https://github.com/ayn2op/discordo
cd discordo
go build .
```

#### Tags

- `no_voice` to build without voice support.
- `no_spoof_tls_fingerprint` to build without TLS fingerprint spoofing.
- `no_syntax_highlighting` to build without syntax highlighting in code blocks.

## Usage

### Password (UI)

1. Run the `discordo` executable with no arguments.

2. Open the "Password" tab, enter your email address or E.164-formatted phone number and password, then click the "Login" button.

### QR (UI)

1. Run the `discordo` executable with no arguments.

2. Click on the "Login with QR" button.

3. Follow the instructions in the QR Login screen.

### Token (UI)

1. Run the `discordo` executable with no arguments.

2. Enter your token and click on the "Login" button to save it.

### Token (environment variable)

Set the value of the `DISCORDO_TOKEN` environment variable to the authentication token to log in with.

```sh
DISCORDO_TOKEN="OTI2MDU5NTQxNDE2Nzc5ODA2.Yc2KKA.2iZ-5JxgxG-9Ub8GHzBSn-NJjNg" discordo
```

### Commands

Typed in the message box and sent with enter:

- `/gif cats` searches GIFs, as Discord's GIF picker does, and shows the one selected beside the list; enter sends it. `/gif` alone shows the GIFs trending.

### Account export

The settings page (ctrl+s) exports the account into a folder beside the config file: the profile and avatar, the
servers with links to join them again, the friends and groups, and discordo's own settings, with `checklist.md` of
what to do to bring it all back. "Export my account, making invites to my servers" also makes a link that does not
expire to each server that has no vanity link, where you may invite. "Import profile from an export" sets the
display name, avatar, About me and pronouns of the account logged in; the servers and friends are added back by
hand from the checklist, as Discord takes doing that in bulk for spam.

## Configuration

The configuration file allows you to configure and customize the behavior, keybindings, and theme of the application.

- Unix: `$XDG_CONFIG_HOME/discordo/config.toml` or `$HOME/.config/discordo/config.toml`
- Darwin: `$HOME/Library/Application Support/discordo/config.toml`
- Windows: `%AppData%/discordo/config.toml`

A `config.toml` in the same folder as the program is used before these, so a copy of discordo kept in a folder of its own, such as the Windows release, can keep its settings beside it.

Use `-config-path` to change the path.

Discordo uses the default configuration if a configuration file is not found in the aforementioned path; however, the default configuration file is not written to the path. [The configuration reference can be found here](./docs/config.md).

## Logs

The log file is replaced on each run.

- Unix: `$XDG_CACHE_HOME/discordo/logs.txt` or `$HOME/.cache/discordo/logs.txt`
- Darwin: `$HOME/Library/Caches/discordo/logs.txt`
- Windows: `%LocalAppData%/discordo/logs.txt`

Use `-log-path` to change the path, and `-log-level` (`debug`, `info`, `warn`, or `error`) to set the verbosity of what is logged.

## License

Copyright (C) 2025-present ayn2op

This project is licensed under the GNU General Public License v3.0 (GPL-3.0).
See the [LICENSE](./LICENSE) file for the full license text.

# Disclaimer

> [!IMPORTANT]
> Automated user accounts or "self-bots" are against Discord's Terms of Service. I am not responsible for any loss caused by using "self-bots" or Discordo.
