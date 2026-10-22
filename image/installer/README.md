# Installer status

There is no automated installer in this release. The live ISO boots on
x86_64 hardware and runs from RAM; `/data` and `/var/lib/mazima` need to be
placed on persistent storage manually.

A future installer should only be added once it can:

1. detect and confirm the target disk without touching unrelated disks;
2. partition GPT with a separate `/data` filesystem;
3. install the release bundle and bootloader;
4. preserve `/data` across reinstalls;
5. be tested in a VM and on real hardware before being advertised.
