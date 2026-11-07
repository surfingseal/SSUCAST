package main

import "testing"

func TestDeploymentBindings(t *testing.T) {
	for _, v := range []struct {
		env          map[string]string
		addr, origin string
		bad          bool
	}{
		{map[string]string{}, "127.0.0.1:8080", "", false},
		{map[string]string{"WEB_ADDR": "0.0.0.0:8080"}, "", "", true},
		{map[string]string{"WEB_DEPLOYMENT": "render", "PORT": "10000", "RENDER_EXTERNAL_URL": "https://ssucast.onrender.com"}, "0.0.0.0:10000", "https://ssucast.onrender.com", false},
		{map[string]string{"WEB_DEPLOYMENT": "render", "RENDER_EXTERNAL_URL": "http://unsafe.example"}, "", "", true},
		{map[string]string{"WEB_DEPLOYMENT": "render", "RENDER_EXTERNAL_URL": "https://site.example/path"}, "", "", true},
		{map[string]string{"WEB_DEPLOYMENT": "render", "PORT": "bad", "RENDER_EXTERNAL_URL": "https://site.example"}, "", "", true},
		{map[string]string{"WEB_DEPLOYMENT": "render"}, "", "", true},
	} {
		a, o, e := webSettings(func(k string) string { return v.env[k] })
		if v.bad {
			if e == nil {
				t.Fatal("unsafe deployment config accepted")
			}
		} else if e != nil || a != v.addr || o != v.origin {
			t.Fatal(a, o, e)
		}
	}
}
