package config

func GetForwardTarget(source string) string {
    switch source {
    case "github":
        return "http://localhost:9000/github"
    case "stripe":
        return "http://localhost:9000/stripe"
    default:
        return ""
    }
}