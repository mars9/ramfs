/*
Usage: ramfs [options]

Ramfs starts a 9P2000 file server keeping all files in memory. The
filesystem is entirely maintained in memory, no external storage is
used. File data is allocated in 2 MiB blocks.

The root of the filesystem is owned by the user who invoked ramfs and
is created with Read, Write and Execute permissions for the owner and
Read and Execute permissions for everyone else (0755). Ramfs create
the necessary directories and files in /adm/ctl, /adm/group and
/<hostowner>.

Options:
  -addr="localhost:5640": service listen address
  -hostowner="mason": hostowner (default: $USER)
  -net="tcp": stream-oriented network
*/
package main
