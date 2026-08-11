import type { Messages } from "../../types"

const common = {
  // common
  "common.name": "Name",
  "common.displayName": "Display Name",
  "common.status": "Status",
  "common.created": "Created",
  "common.createdBy": "Created By",
  "common.total": "{count} total",
  "common.delete": "Delete",
  "common.description": "Description",
  "common.edit": "Edit",
  "common.cancel": "Cancel",
  "common.confirm": "Confirm",
  "common.confirmByName.label": 'Type the name "{name}" to confirm this action.',
  "common.confirmByName.mismatch": "Name does not match.",
  "common.save": "Save",
  "common.search": "Search",
  "common.noData": "No data",
  "common.noOptions": "No options available",
  "common.actions": "Actions",
  "common.active": "Active",
  "common.inactive": "Inactive",
  "common.all": "All",
  "common.phone": "Phone",
  "common.password": "Password",
  "common.previous": "Previous",
  "common.next": "Next",
  "common.pageSize": "Per page",
  "common.firstPage": "First page",
  "common.lastPage": "Last page",
  "common.gotoPage": "Page {page}",
  "common.jumpTo": "Go to",
  "common.pageUnit": "",
  "common.jumpToPageInput": "Jump to page",
  "common.noSearchResults": "No matching results found.",
  "common.reset": "Reset",
  "common.current": "Current",
  "common.loadError": "Failed to load.",
  "common.retry": "Retry",
  "common.updated": "Updated",

  // auth
  "auth.authenticating": "Authenticating...",
  "auth.missingCode": "Missing authorization code",

  // login
  "login.title": "Vraxel Console",
  "login.username": "Username",
  "login.password": "Password",
  "login.usernamePlaceholder": "Enter username",
  "login.passwordPlaceholder": "Enter password",
  "login.signIn": "Sign In",
  "login.noAccount": "Don't have an account?",
  "login.createAccount": "Create account",
  "login.orContinueWith": "Or continue with",
  "login.social.github": "Continue with GitHub",
  "login.social.google": "Continue with Google",

  // register
  "register.title": "Create Account",
  "register.email": "Email",
  "register.emailPlaceholder": "Enter email",
  "register.displayName": "Display Name",
  "register.displayNamePlaceholder": "Enter display name (optional)",
  "register.confirmPassword": "Confirm Password",
  "register.confirmPasswordPlaceholder": "Re-enter password",
  "register.submit": "Sign Up",
  "register.haveAccount": "Already have an account?",
  "register.signIn": "Sign in",

  // nav
  "nav.iam": "IAM",
  "nav.workspaces": "Workspaces",
  "nav.namespaces": "Namespaces",
  "nav.users": "Users",
  "nav.roles": "Roles",
  "nav.audit": "Audit",
  "nav.auditLogs": "Audit Logs",
  "nav.rolebindings": "Role Bindings",
  "nav.kube": "Kubernetes",
  "nav.hosts": "Hosts",
  "nav.apiDocs": "API Docs",
  "nav.searchPlaceholder": "Search menu ({key})",
  "nav.searchNoMatch": "No match",

  // overview

  // scope selector
  "scope.allWorkspaces": "All Workspaces",
  "scope.allNamespaces": "All Namespaces",
  "scope.selectWorkspace": "Select workspace",
  "scope.selectNamespace": "Select namespace",

  // permission verb wildcards
  "perm.group.all": "All Permissions",
  "perm.verb.list": "All list (*:list)",
  "perm.verb.get": "All get (*:get)",
  "perm.verb.create": "All create (*:create)",
  "perm.verb.update": "All update (*:update)",
  "perm.verb.patch": "All patch (*:patch)",
  "perm.verb.delete": "All delete (*:delete)",
  "perm.verb.deleteCollection": "All batch delete (*:deleteCollection)",

  // permission verb groups
  "perm.verbGroup.read": "Read",
  "perm.verbGroup.create": "Create",
  "perm.verbGroup.update": "Update",
  "perm.verbGroup.delete": "Delete",

  // error
  "error.400.title": "Bad Request",
  "error.400.desc": "The request could not be understood. Please try again.",
  "error.401.title": "Unauthorized",
  "error.401.desc": "Please sign in to continue.",
  "error.403.title": "Forbidden",
  "error.403.desc": "You don't have permission to access this page.",
  "error.404.title": "Not Found",
  "error.404.desc": "The page you are looking for does not exist.",
  "error.500.title": "Server Error",
  "error.500.desc": "Something went wrong. Please try again later.",
  "error.backHome": "Back to Home",
  "error.switchAccount": "Switch Account",

  // login errors
  "login.error.invalidCredentials": "Invalid username or password",
  "login.error.accountInactive": "Account has been deactivated",
  "login.error.tooManyAttempts": "Too many failed attempts; temporarily locked, try again later.",
  "login.error.sessionExpired": "Session expired, redirecting...",
  "login.error.failed": "Login failed, please try again",

  // register errors
  "register.error.passwordMismatch": "The two passwords do not match",
  "register.error.conflict": "Username or email already exists",
  "register.error.tooManyAttempts": "Too many registration attempts; try again later.",
  "register.error.failed": "Registration failed, please try again",

  // api errors
  "api.error.badRequest": "Bad request",
  "api.error.invalidJSONBody": "Invalid request body",
  "api.error.notFound": "Not found",
  "api.error.conflict": "Conflict",
  "api.error.badGateway": "Upstream service error, please retry",
  "api.error.gatewayTimeout": "Upstream service timed out, please retry",
  "api.error.networkError": "Cannot reach the server. Check your network or retry shortly.",
  "api.error.sessionExpired": "Session expired, redirecting to login",
  "api.error.memberLimitExceeded": "Member limit exceeded for this namespace",
  "api.error.cannotDeleteWorkspace":
    "Cannot delete workspace: it still contains active resources, please remove them first",
  "api.error.cannotDeleteNamespace":
    "Cannot delete namespace: it still contains active resources, please remove them first",
  "blockingResource.host": "Host",
  // placement scheduler errors (mirrors placement.ReasonFor in the Go backend).
  "api.error.cannotRemoveOwner": "Cannot remove the owner from this resource",
  "api.error.oldPasswordIncorrect": "Current password is incorrect",
  "api.error.forbidden": "You do not have permission to perform this action",
  "api.error.internalError": "Internal server error, please try again later",
  "api.error.timeout": "Request timed out, please try again later",
  "api.error.cannotDeleteBuiltinRole": "Cannot delete built-in role",
  "api.error.cannotDeleteRoleWithBindings": "Cannot delete role with active bindings",
  "api.error.valueTooLong": "Input value is too long",

  // validation errors
  "api.validation.formHasErrors": "Form has {count} field(s) failing validation:",
  "api.validation.required": "{field} is required",
  "api.validation.username.format":
    "Username must be 3-50 characters of letters, digits, underscores, or hyphens",
  "api.validation.email.format": "Please enter a valid email address",
  "api.validation.phone.format": "Please enter a valid mobile number (e.g. 13800138000)",
  "api.validation.password.length": "Password must be 8-72 characters",
  "api.validation.password.uppercase": "Password must contain at least one uppercase letter",
  "api.validation.password.lowercase": "Password must contain at least one lowercase letter",
  "api.validation.password.digit": "Password must contain at least one digit",
  "api.validation.name.format":
    "Name must be 3-50 letters, digits, hyphens, or underscores, starting and ending with a letter or digit",
  "api.validation.rackCapacity.min": "Rack capacity must be >= 0",
  "api.validation.rackCapacity.range": "Rack capacity must be an integer between 0 and 10000",
  "api.validation.uHeight.range": "U height must be an integer between 0 and 100",
  "api.validation.status.format": "Status must be 'active' or 'inactive'",
  "api.validation.username.taken": "This username is already taken",
  "api.validation.email.taken": "This email is already taken",
  "api.validation.phone.taken": "This phone number is already taken",
  "api.validation.password.hint": "8-72 characters, must include uppercase, lowercase, and a digit",
  "api.validation.name.networkFormat":
    "Must be 3-50 lowercase alphanumeric characters or hyphens, starting and ending with alphanumeric",
  "api.validation.cidr.format": "Please enter a valid CIDR (e.g. 10.0.0.0/24)",
  "api.validation.ip.format": "Please enter a valid IP address",
  "api.validation.image.format":
    "Invalid image reference format (e.g. nginx:1.25, registry.example.com/repo:tag)",
  "api.validation.gateway.notInRange": "Gateway is not within the CIDR range",
  "api.validation.cidr.overlap": "CIDR overlaps with an existing subnet",
  "api.validation.cidr.notWithinNetwork": "Subnet CIDR is not within network CIDR range",
  "api.validation.description.tooLong": "Description is too long",
  "api.validation.minLength": "Must be at least {min} characters",
  "api.validation.maxLength": "Must be at most {max} characters",
  "api.validation.intRange": "Must be between {min} and {max}",
  "api.validation.memberRange": "Must be between 0 and 1,000,000",
  "api.validation.subnetRange": "Must be between 1 and 50",
  "api.validation.nonNegativeInt": "Must be a non-negative integer",
  "api.validation.integer.format":
    'Must be a non-negative integer without leading zeros ("0" alone is allowed)',
  "api.validation.positive": "Must be a positive number",
  "api.validation.port.range": "Port must be between 1 and 65535",
  "api.validation.lat.range": "Latitude must be between -90 and 90",
  "api.validation.lng.range": "Longitude must be between -180 and 180",
  "api.validation.tooLarge": "Value is too large",
  "api.validation.path.absolute": "Must be an absolute path (starting with /)",
  "api.validation.bufferPool.format": "Must be a number followed by M or G (e.g. 256M, 4G)",
  "api.validation.resources.cpu.format":
    "Quantity must be a positive integer; decimals such as 0.5 Cores are auto-converted to a smaller unit (500 m) when the field loses focus.",
  "api.validation.resources.memory.format":
    "Quantity must be a positive integer; decimals such as 0.5 Gi are auto-converted to a smaller unit (512 Mi) when the field loses focus.",
  "api.validation.resources.volumeSize.format":
    "Must be a number suffixed with Mi, Gi, or Ti (e.g. 1Gi, 500Mi, 2Ti)",
  "api.validation.connRange": "Must be between 1 and 100,000",
  "api.validation.binaryChoice": "Must be 0 or 1",
  "api.validation.hostnameOrIpRequired": "Hostname or IP is required",

  // batch delete

  // action feedback
  "action.createSuccess": "Created successfully",
  "action.updateSuccess": "Updated successfully",
  "action.deleteSuccess": "Deleted successfully",
  "action.resetPasswordSuccess":
    "Password reset. Refresh tokens revoked; the user's existing access token expires within 1 hour.",

  // user menu
  "userMenu.changePassword": "Change Password",
  "userMenu.logout": "Sign Out",
  "userMenu.oldPassword": "Current Password",
  "userMenu.newPassword": "New Password",
  "userMenu.confirmPassword": "Confirm New Password",
  "userMenu.passwordMismatch": "Passwords do not match",
  "userMenu.passwordSameAsOld": "New password must be different from current password",

  // task terminal

  // Shared probe / handler editor
} satisfies Messages

export default common
