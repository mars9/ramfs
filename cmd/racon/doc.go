/*
Usage: racon [options] cmd [option] args...

9P2000 client that can access a single file on a 9P2000 server. It can
be useful for manual interaction with a 9P2000 server or for accessing
simple 9P2000 services from within scripts.

Options:
  -addr="localhost:5640": service network address
  -aname="": attach to the file system named aname
  -crypt=false: use AES en-/decryption
  -d=false: make directories
  -l=false: use a long listing format
  -snappy=false: use snappy en-/decompression
  -uname="$USER": username (default: $USER)

Commands:
  chgrp group file... - change file group
  chmod mode file...  - change file modes
  create [-d] file... - make directories or files
  ls [-l] file        - list contents of directory of file
  noop                - send attach request
  read file...        - write the contents of file to stdout
  stat file...        - write status information to stdout
  write file          - read stdin and write contents to file
*/
package main
