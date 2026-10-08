package v1_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryModuleIsRegistered 每个 newXxxModule 构造函数都必须出现在 Modules() 清单里。
//
// 清单那一行是本套模块体系**唯一**需要手改的地方，而漏掉它是**静默失效**：
// 接口全部 404，但不报错、不 panic，路由数量断言也抓不到（它只校验已知的 8 个）。
//
// 所以用一个 AST 测试直接比对「定义了哪些模块构造函数」与「清单里注册了哪些」。
func TestEveryModuleIsRegistered(t *testing.T) {
	defined, registered := scanModuleFactories(t)

	if len(defined) == 0 {
		t.Fatal("没有扫描到任何 newXxxModule 构造函数 —— 测试本身可能失效了，请检查 scanModuleFactories")
	}

	for name := range defined {
		if !registered[name] {
			t.Errorf("模块 %s 未在 Modules() 清单中注册 —— 它的路由不会生效（静默失效）", name)
		}
	}
	for name := range registered {
		if !defined[name] {
			t.Errorf("Modules() 引用了不存在的模块构造函数 %s", name)
		}
	}
}

// scanModuleFactories 解析本包源码（跳过测试文件），返回
// ① 定义的全部模块构造函数 ② Modules() 里实际调用的那些。
func scanModuleFactories(t *testing.T) (defined, registered map[string]bool) {
	t.Helper()

	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("读取包目录失败: %v", err)
	}

	defined = map[string]bool{}
	registered = map[string]bool{}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("解析 %s 失败: %v", name, err)
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil {
				continue // 只看包级函数，方法不算
			}
			switch {
			case isModuleFactory(fn.Name.Name):
				defined[fn.Name.Name] = true
			case fn.Name.Name == "Modules":
				collectCalledFactories(fn, registered)
			}
		}
	}
	return defined, registered
}

// isModuleFactory 判断函数名是否形如 newXxxModule（模块构造函数）
func isModuleFactory(name string) bool {
	return name != "newModule" &&
		strings.HasPrefix(name, "new") &&
		strings.HasSuffix(name, "Module")
}

// collectCalledFactories 收集 Modules() 函数体里调用的模块构造函数
func collectCalledFactories(fn *ast.FuncDecl, out map[string]bool) {
	if fn.Body == nil {
		return
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id, ok := call.Fun.(*ast.Ident); ok && isModuleFactory(id.Name) {
			out[id.Name] = true
		}
		return true
	})
}
