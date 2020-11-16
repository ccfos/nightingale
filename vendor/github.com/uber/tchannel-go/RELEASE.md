Release process
===============

This document outlines how to create a release of tchannel-go

1.  Set up some environment variables for use later.

    ```
    # This is the version being released.
    $ VERSION=1.8.0
    ```

2.  Make sure you have the latest dev and create a branch off it.

    ```
    $ git checkout dev
    $ git pull
    $ git checkout -b release
    ```

3.  Update the `CHANGELOG.md` and `version.go` files.

    ```
    $ go run ./scripts/vbumper/main.go --version $VERSION
    ```

4.  Clean up the `CHANGELOG.md` to only mention noteworthy changes for users.

5.  Commit changes and create a PR against `dev` to prepare for release.

6.  Once the release PR has been accepted, run the following to release.

    ```
    $ git checkout master
    $ git pull
    $ git merge dev
    $ git tag -a "v$VERSION" -m "v$VERSION"
    $ git push origin master v$VERSION
    ```

7.  Go to <https://github.com/uber/tchannel-go/tags> and edit the release notes.
    Copy changelog entries for this release and set the name to `v$VERSION`.

8.  Switch back to development.

    ```
    $ git checkout dev
    $ git merge master
    $ go run ./scripts/vbumper/main.go --version ${VERSION}-dev --skip-changelog
    $ git commit -am "Back to development"
    $ git push
    ```
