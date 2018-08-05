
Lectio Daemon
=============

This is a work in progress, talk with Shahid to learn more about it before documentation is completed.

Build instructions
==================

Get the latest code

    git clone git@github.com:lectio/lectiod.git

Initialize the development environment

    make init-devl

If you ever need to clean the dependencies and reset them:

    make clean
    make init-devl

The key.go source file is broken, with the following message:

    vendor/github.com/ipfs/go-datastore/key.go:257:41: not enough arguments in call to uuid.Must

So, temporarily need to patch key.go by using the github.com/google/uuid package instead of github.com/satori/go.uuid package.

    dep ensure --add "github.com/google/uuid"
    vi vendor/github.com/ipfs/go-datastore/key.go

Now [apply this fix](https://github.com/ipfs/go-datastore/commit/2fa1cdde8d95600fd062738e7d43a2acde18b648) to key.go and save.

Testing
=======

As a server:

    make

Just the test suite:

    make test