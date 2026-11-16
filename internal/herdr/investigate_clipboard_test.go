package herdr

import (
 "context"
 "encoding/json"
 "strings"
 "testing"
 "time"
 "rove/internal/core"
)

func TestInvestigateRemoteClipboardStream(t *testing.T) {
 cl, err := core.Connect(""); if err != nil { t.Fatal(err) }; defer cl.Close()
 ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second); defer cancel()
 raw, err := cl.Call(ctx,"terminal.run",map[string]any{"on":"gpu", "argv":[]string{"sh","-c",`read x; printf '\033]52;c;cm92ZS1jbGlwYm9hcmQtdGVzdA==\007'; printf 'DONE'; sleep 1`},"keep":false}); if err!=nil {t.Fatal(err)}
 var data struct{Address string `json:"address"`}; if err=json.Unmarshal(raw,&data); err!=nil {t.Fatal(err)}
 defer cl.Call(context.Background(),"terminal.kill",map[string]any{"terminal":data.Address})
 ch,_,err:=cl.Attach(ctx,data.Address,80,24,map[string]any{}); if err!=nil {t.Fatal(err)}; defer cl.Detach(ch)
 stream,err:=cl.Stream(ch); if err!=nil {t.Fatal(err)}; cl.Credit(ch,1<<20); cl.Input(ch,[]byte("go\n"))
 var out strings.Builder
 for {select {case item:=<-stream: if item.Kind==core.FrameData {out.Write(item.Data); if strings.Contains(out.String(),"DONE") {t.Logf("stream = %q",out.String()); if !strings.Contains(out.String(),"\x1b]52;") {t.Fatal("OSC 52 missing")}; return}}; case <-ctx.Done(): t.Fatalf("timeout; stream = %q",out.String())}}
}
