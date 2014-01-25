# RAMFS

Ramfs starts a 9P2000 file server keeping all files in memory. The
filesystem is entirely maintained in memory, no external storage is
used. File data is allocated in 2 MiB blocks.

The root of the filesystem is owned by the user who invoked ramfs and
is created with Read, Write and Execute permissions for the owner and
Read and Execute permissions for everyone else (0755). Ramfs create
the necessary directories and files in /adm/ctl, /adm/group and
/<hostowner>.

# 9P2000

A 9P2000 server is an agent that provides one or more hierarchical
file systems -- file trees -- that may be accessed by processes. A
server responds to requests by clients to navigate the hierarchy, and
to create, remove, read, and write files.

### References 

* [http://plan9.bell-labs.com/magic/man2html/5/0intro](http://plan9.bell-labs.com/magic/man2html/5/0intro "Intro")
* [http://plan9.bell-labs.com/magic/man2html/5/attach](http://plan9.bell-labs.com/magic/man2html/5/attach "Attach")
* [http://plan9.bell-labs.com/magic/man2html/5/clunk](http://plan9.bell-labs.com/magic/man2html/5/clunk "Clunk")
* [http://plan9.bell-labs.com/magic/man2html/5/error](http://plan9.bell-labs.com/magic/man2html/5/error "Error")
* [http://plan9.bell-labs.com/magic/man2html/5/flush](http://plan9.bell-labs.com/magic/man2html/5/flush "Flush")
* [http://plan9.bell-labs.com/magic/man2html/5/open](http://plan9.bell-labs.com/magic/man2html/5/open "Open")
* [http://plan9.bell-labs.com/magic/man2html/5/read](http://plan9.bell-labs.com/magic/man2html/5/read "Read")
* [http://plan9.bell-labs.com/magic/man2html/5/remove](http://plan9.bell-labs.com/magic/man2html/5/remove "Remove")
* [http://plan9.bell-labs.com/magic/man2html/5/stat](http://plan9.bell-labs.com/magic/man2html/5/stat "Stat")
* [http://plan9.bell-labs.com/magic/man2html/5/version](http://plan9.bell-labs.com/magic/man2html/5/version "Version")
* [http://plan9.bell-labs.com/magic/man2html/5/walk](http://plan9.bell-labs.com/magic/man2html/5/walk "Walk")

# Usage

To add a new user with name and id gnot and create his home directory:

    echo uname gnot gnot | racon write /adm/group

To create a new group sys (with no home directory) and add gnot to it:

    echo uname sys :sys | racon write /adm/group
    echo uname sys +gnot | racon write /adm/group

Listen manages the network addresses at which fossil is listening.

    echo listen tcp localhost:5641 | racon write /adm/ctl

