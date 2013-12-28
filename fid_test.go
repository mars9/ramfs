package ramfs

import (
	"fmt"
	"testing"

	"code.google.com/p/goplan9/plan9"
)

func TestPermission(t *testing.T) {
	unknownUser := user{"unknownUser", "unknownUser", member{}}
	unknownGroup := user{"unknownGroup", "unknownGroup", member{}}
	adm := user{"adm", "adm", member{}}
	none := user{"none", "none", member{}}
	glenda := user{"glenda", "glenda", member{}}
	var tests = struct {
		perm []plan9.Perm
		test [][]struct {
			uid    user
			result [3]error
		}
	}{
		perm: []plan9.Perm{0666, 0640, 0400, 0604, 0777, 0620},
		test: [][]struct {
			uid    user
			result [3]error
		}{
			{
				{adm, [3]error{nil, nil, nil}},
				{glenda, [3]error{nil, nil, nil}},
				{none, [3]error{nil, nil, nil}},
				{unknownUser, [3]error{errPerm, errPerm, errPerm}},
				{unknownGroup, [3]error{errPerm, errPerm, errPerm}},
			}, {
				{adm, [3]error{nil, nil, nil}},
				{glenda, [3]error{errPerm, errPerm, nil}},
				{none, [3]error{errPerm, errPerm, errPerm}},
				{unknownUser, [3]error{errPerm, errPerm, errPerm}},
				{unknownGroup, [3]error{errPerm, errPerm, errPerm}},
			}, {
				{adm, [3]error{errPerm, errPerm, nil}},
				{glenda, [3]error{errPerm, errPerm, errPerm}},
				{none, [3]error{errPerm, errPerm, errPerm}},
				{unknownUser, [3]error{errPerm, errPerm, errPerm}},
				{unknownGroup, [3]error{errPerm, errPerm, errPerm}},
			}, {
				{adm, [3]error{nil, nil, nil}},
				{glenda, [3]error{errPerm, errPerm, nil}},
				{none, [3]error{errPerm, errPerm, nil}},
				{unknownUser, [3]error{errPerm, errPerm, errPerm}},
				{unknownGroup, [3]error{errPerm, errPerm, errPerm}},
			}, {
				{adm, [3]error{nil, nil, nil}},
				{glenda, [3]error{nil, nil, nil}},
				{none, [3]error{nil, nil, nil}},
				{unknownUser, [3]error{errPerm, errPerm, errPerm}},
				{unknownGroup, [3]error{errPerm, errPerm, errPerm}},
			}, {
				{adm, [3]error{nil, nil, nil}},
				//{glenda, [3]error{nil, errPerm, errPerm}},
				//{none, [3]error{errPerm, errPerm, errPerm}},
				//{unknownUser, [3]error{errPerm, errPerm, errPerm}},
				//{unknownGroup, [3]error{errPerm, errPerm, errPerm}},
			},
		},
	}

	// root permission == 0755|plan9.DMDIR
	fs := New("bootes")
	fs.group.groupmap["glenda"] = user{"glenda", "glenda", member{}}
	fs.group.groupmap["adm"].Member["glenda"] = true

	for i, perm := range tests.perm {
		name := fmt.Sprintf("/file-%d", i)
		f, err := fs.root.Create("adm", name, plan9.ORDWR, perm)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		// fix adjusted permissions
		f.dir.Mode = perm

		for j, test := range tests.test[i] {
			mode := uint8(plan9.OWRITE)
			fid := Fid{node: f, uid: test.uid.Name}
			if err := fid.Open(mode); err != test.result[0] {
				t.Fatalf("open write %d:%d: %q: expected %v, got %v",
					i, j, test.uid.Name, test.result[0], err)
			}
			fid.Close()

			fid = Fid{node: f, uid: test.uid.Name}
			mode = uint8(plan9.ORDWR)
			if err := fid.Open(mode); err != test.result[1] {
				t.Fatalf("open rdwr %d:%d: %q: expected %v, got %v",
					i, j, test.uid.Name, test.result[1], err)
			}
			f.Close()

			fid = Fid{node: f, uid: test.uid.Name}
			mode = uint8(plan9.OREAD)
			if err := fid.Open(mode); err != test.result[2] {
				t.Fatalf("open read %d:%d: %q: expected %v, got %v", i, j,
					test.uid.Name, test.result[2], err)
			}
			fid.Close()
		}
		f.Close()
	}
}
