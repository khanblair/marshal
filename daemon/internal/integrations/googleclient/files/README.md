Marshal's own Google OAuth client goes here as `client.json`, written by the release build:

    {"clientId": "...apps.googleusercontent.com", "clientSecret": "..."}

The file is not in git. Without it the build has no built-in Google client and Settings asks the
person for their own (see docs/development.md, section 3.7a).
