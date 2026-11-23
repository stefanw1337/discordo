### About antivirus and browser warnings

The files are not code-signed, as discordo is made privately and signing costs money. A browser or antivirus program may therefore warn about them, as it does about many new, unsigned programs, Go programs and Discord clients most of all. Firefox, for example, asks Google Safe Browsing about each new file, and a file few have downloaded yet has no reputation.

- Every file is built here on GitHub from the source in this repository, by the workflow in `.github/workflows/release.yml`, which anyone can read and run.
- `SHA256SUMS.txt` lists the checksum of each file, to check that the one downloaded is the one built.
- discordo reads keys while another window has the focus only for push to talk, which is off until a key is chosen for it, and only while in a voice channel; and for `watch_paste_key`, which is off unless turned on. Antivirus programs can take that for a keylogger.
- To see what each antivirus program says of a file, upload it to [VirusTotal](https://www.virustotal.com).

