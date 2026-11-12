<p align="center">
  <a href="https://rudderstack.com/">
    <img src="https://user-images.githubusercontent.com/59817155/121357083-1c571300-c94f-11eb-8cc7-ce6df13855c9.png">
  </a>
</p>

<p align="center"><b>The Customer Data Platform for Developers</b></p>

<p align="center">
  <b>
    <a href="https://rudderstack.com">Website</a>
    ·
    <a href="">Documentation</a>
    ·
    <a href="https://rudderstack.com/join-rudderstack-slack-community">Community Slack</a>
  </b>
</p>

---

# \*\*Repo Name\*\*

\*\*Repo description\*\*

## Overview

\*\*Describe what the software does.\*\*

## Features

\*\*Describe the key features, if necessary.\*\*

## Getting started

\*\*Describe how to use the software.\*\*

## Bulk upload protocol

`NewBulkService(session)` continues to use SOAP by default. Opt in to the REST
bulk upload URL and status endpoints with:

```go
service := bingads.NewBulkService(session, bingads.WithProtocol(bingads.ProtocolREST))
url, err := service.GetBulkUploadUrl()
// Use url.RequestId with service.GetBulkUploadStatus(url.RequestId).
```

REST requires `session.TokenSource` (OAuth); it does not support SOAP's
username/password authentication. `UploadBulkFile` is unchanged. For tests,
`bingads.WithRESTBaseURL(server.URL)` overrides the default REST base URL.

## Contribute

We would love to see you contribute to RudderStack. Get more information on how to contribute [**here**](CONTRIBUTING.md).

## License

The RudderStack \*\*software name\*\* is released under the [**MIT License**](https://opensource.org/licenses/MIT).
