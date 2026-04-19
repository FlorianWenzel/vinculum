Output vnclm.gif

Env PATH "/Users/florianwenzel/go/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
Env KUBECONFIG "/Users/florianwenzel/.kube/config"
Env HOME "/Users/florianwenzel"
Env TERM "xterm-256color"
Env STARSHIP_CONFIG "/Users/florianwenzel/.config/starship.toml"

Set Shell "zsh"
Set FontSize 15
Set Width 1200
Set Height 1100
Set Theme "Catppuccin Mocha"
Set TypingSpeed 32ms
Set PlaybackSpeed 1.0
Set Padding 20

Hide
Type "cd ~/projects/vinculum"
Enter
Type "kubectx nova >/dev/null 2>&1; kubens vinculum-system >/dev/null 2>&1"
Enter
Type `eval "$(starship init zsh)"`
Enter
Sleep 500ms
Type "clear"
Enter
Sleep 1500ms
Show

Type "vnclm get agents"
Sleep 300ms
Enter
Sleep 2200ms

Type "vnclm create agent"
Sleep 300ms
Enter
Sleep 3500ms

Type "seven-of-nine"
Sleep 900ms
Enter
Sleep 1400ms

Type "azure/gpt-4o"
Sleep 900ms
Enter
Sleep 1400ms

Enter
Sleep 1300ms

Type "You are Seven of Nine. Answer with cold Borg efficiency in one sentence."
Sleep 1200ms
Enter
Sleep 1300ms

Enter
Sleep 1200ms

Enter
Sleep 1200ms

Enter
Sleep 4500ms

Type "vnclm get agents"
Sleep 300ms
Enter
Sleep 2800ms

Type `vnclm run --agent locutus --timeout 45 "Write a haiku about the Borg collective."`
Sleep 300ms
Enter
Sleep 15s

Type "vnclm get tasks"
Sleep 300ms
Enter
Sleep 3000ms
