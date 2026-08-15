Drop any root CA (`.crt`, PEM text) that has to be trusted during the image
build here — a corporate TLS-inspecting proxy, or antivirus HTTPS scanning.
Without it `pip install` fails with `CERTIFICATE_VERIFY_FAILED`.

Export one from the Windows store:

```powershell
$c = Get-ChildItem Cert:\LocalMachine\Root |
     Where-Object { $_.Subject -like "*<name of the root>*" } | Select -First 1
"-----BEGIN CERTIFICATE-----`n" +
  [Convert]::ToBase64String($c.RawData,'InsertLineBreaks') +
  "`n-----END CERTIFICATE-----" | Set-Content ./that-root.crt -Encoding ascii
```

Find which root is intercepting you: `openssl s_client -connect pypi.org:443`
and look at the issuer. The node/web image needs the same treatment only if
`npm ci` starts failing too.
