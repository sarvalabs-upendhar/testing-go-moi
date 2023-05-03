osascript -e 'tell app "Terminal"
   do script "moichain bootnode --port 5050"
    Repeat with LoopVar from 0 to 14
     do script ("cd /Users/rahullenkala/go/src/github.com/moichain && moichain server --data-dir test_" & LoopVar & " --network-size 15 --mtq 0.5 --skip-genesis=\"false\"")
      end repeat
      set bounds of windows to {100, 100, 1800, 600}
       end tell'