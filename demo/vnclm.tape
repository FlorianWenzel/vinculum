Output vnclm.gif

Env PATH "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
Env KUBECONFIG "/Users/florianwenzel/.kube/config"
Env HOME "/Users/florianwenzel"
Env TERM "xterm-256color"

Set Shell "zsh"
Set FontSize 15
Set Width 1200
Set Height 720
Set Theme "Catppuccin Mocha"
Set TypingSpeed 30ms
Set PlaybackSpeed 1.0
Set Padding 20

Hide
Type "kubectx nova >/dev/null 2>&1; kubens vinculum-system >/dev/null 2>&1; clear"
Enter
Sleep 1s
Show

Type "vnclm list agents"
Sleep 150ms
Enter
Sleep 2s

Type "vnclm ctx set-agent locutus"
Sleep 150ms
Enter
Sleep 800ms

Type 'vnclm run --fresh --timeout 60 "Compose a haiku about the Borg collective."'
Sleep 150ms
Enter
Sleep 16s

Type "vnclm ctx set-agent seven-of-nine"
Sleep 150ms
Enter
Sleep 800ms

Type 'vnclm run --fresh --timeout 60 "Explain, in one sentence, why resistance is futile."'
Sleep 150ms
Enter
Sleep 14s

Type "vnclm list tasks"
Sleep 150ms
Enter
Sleep 3s
