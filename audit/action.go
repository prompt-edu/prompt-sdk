package audit

import "strings"

var methodVerb = map[string]string{
	"POST":   "Created",
	"PUT":    "Updated",
	"PATCH":  "Updated",
	"DELETE": "Deleted",
}

func isMutating(method string) bool {
	_, ok := methodVerb[method]
	return ok
}

// deriveAction produces a readable-enough default label from the HTTP method
// and the matched route template, e.g. ("POST", "/api/.../slots") -> "Created
// slot". Developers override cryptic cases with audit.Describe.
func deriveAction(method, routeTemplate string) string {
	verb := methodVerb[method]
	if verb == "" {
		verb = method
	}
	resource := resourceFromTemplate(routeTemplate)
	if resource == "" {
		return verb
	}
	return verb + " " + resource
}

// resourceFromTemplate returns the last non-parameter path segment, humanized
// (dashes/underscores to spaces, naive singularization).
func resourceFromTemplate(routeTemplate string) string {
	segments := strings.Split(routeTemplate, "/")
	for i := len(segments) - 1; i >= 0; i-- {
		seg := segments[i]
		if seg == "" || strings.HasPrefix(seg, ":") || strings.HasPrefix(seg, "*") {
			continue
		}
		seg = strings.ReplaceAll(seg, "-", " ")
		seg = strings.ReplaceAll(seg, "_", " ")
		return singularize(strings.ToLower(seg))
	}
	return ""
}

func singularize(word string) string {
	// Only strip a trailing plural "s". Keep singular nouns that naturally end in
	// "s" — double-s ("class"), "-us" ("status", "campus"), and "-is"
	// ("analysis") — so route segments don't become "clas"/"statu"/"analysi".
	if len(word) > 1 && strings.HasSuffix(word, "s") &&
		!strings.HasSuffix(word, "ss") &&
		!strings.HasSuffix(word, "us") &&
		!strings.HasSuffix(word, "is") {
		return strings.TrimSuffix(word, "s")
	}
	return word
}
