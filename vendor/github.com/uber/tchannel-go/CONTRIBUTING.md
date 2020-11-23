Contributing
============

We'd love your help making tchannel-go great!

## Getting Started

TChannel uses [glide](https://github.com/Masterminds/glide) to manage
dependencies.
To get started:

```bash
go get github.com/uber/tchannel-go
make install_glide
make  # tests should pass
```

## Making A Change

*Before making any significant changes, please [open an
issue](https://github.com/uber/tchannel-go/issues).* Discussing your proposed
changes ahead of time will make the contribution process smooth for everyone.

Once we've discussed your changes and you've got your code ready, make sure
that tests are passing (`make test` or `make cover`) and open your PR! Your
pull request is most likely to be accepted if it:

* Includes tests for new functionality.
* Follows the guidelines in [Effective
  Go](https://golang.org/doc/effective_go.html) and the [Go team's common code
  review comments](https://github.com/golang/go/wiki/CodeReviewComments).
* Has a [good commit
  message](http://tbaggery.com/2008/04/19/a-note-about-git-commit-messages.html).

## Cutting a Release

* Send a pull request against dev including:
  * update CHANGELOG.md (`scripts/changelog_halp.sh`)
  * update version.go
* Send a pull request for dev into master
* `git tag -m v0.0.0 -a v0.0.0`
* `git push origin --tags`
* Copy CHANGELOG.md fragment into release notes on
  https://github.com/uber/tchannel-go/releases 
