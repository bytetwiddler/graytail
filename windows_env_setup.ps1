<#
PowerShell script to prompt for required environment variables and persist them for the current user.
Run this from a PowerShell prompt (normal user). It sets variables using the User scope and
also updates the current session environment so the values are immediately usable.
#>

$vars = @('GRAYLOG_URL','GRAYLOG_API_TOKEN','GRAYLOG_STREAM_ID')

# Graylog's built-in "All messages" stream ID; used as the default when the
# user leaves GRAYLOG_STREAM_ID blank.
$defaults = @{ 'GRAYLOG_STREAM_ID' = '000000000000000000000001' }

foreach ($v in $vars) {
    $current = [Environment]::GetEnvironmentVariable($v, 'User')
    if ($current) {
        Write-Host "$v already set for user: $current"
        $use = Read-Host "Do you want to overwrite $v? (y/N)"
        if ($use -ne 'y' -and $use -ne 'Y') { continue }
    }

    $default = $defaults[$v]
    if ($default) {
        $val = Read-Host "Enter value for $v (default: $default)"
        if ([string]::IsNullOrWhiteSpace($val)) { $val = $default }
    } else {
        $val = Read-Host "Enter value for $v"
    }
    if ([string]::IsNullOrWhiteSpace($val)) {
        Write-Host "Skipping $v (no value provided)"
        continue
    }

    try {
        [Environment]::SetEnvironmentVariable($v, $val, 'User')
        # update current session as well
        Set-Item -Path "Env:$v" -Value $val
        Write-Host "$v set for current session and persisted for user."
    } catch {
        Write-Error ("Failed to set {0}: {1}" -f $v, $_)
    }
}

Write-Host "Done. You may need to start a new shell or sign out/in for some apps to see the updated variables."
