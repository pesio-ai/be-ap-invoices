package client

import (
	"context"

	identitypb "github.com/pesio-ai/be-lib-proto/gen/go/platform"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// IdentityGRPCClient implements service.IdentityClientInterface against the
// platform identity gRPC service.
//
// NOTE: The identity proto currently has no RPC that returns users by role
// (no ListUsers, no GetUsersWithRole). Until such an RPC is added:
//   - GetUsersWithRole returns an empty slice → approval steps are created
//     unassigned and can be acted on by any user with the required role.
//   - GetUserRoles returns roles derived from the user's permissions.
type IdentityGRPCClient struct {
	client identitypb.IdentityServiceClient
	conn   *grpc.ClientConn
}

// NewIdentityGRPCClient dials the identity gRPC service and returns a client.
func NewIdentityGRPCClient(addr string) (*IdentityGRPCClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &IdentityGRPCClient{
		client: identitypb.NewIdentityServiceClient(conn),
		conn:   conn,
	}, nil
}

// Close releases the underlying gRPC connection.
func (c *IdentityGRPCClient) Close() error {
	return c.conn.Close()
}

// GetUsersWithRole returns user IDs that hold the given role for an entity.
// The identity service does not expose a "list users by role" RPC yet,
// so this always returns an empty slice — steps will be left unassigned.
func (c *IdentityGRPCClient) GetUsersWithRole(ctx context.Context, entityID, role string) ([]string, error) {
	// TODO: call identity ListUsersByRole once that RPC is added to the proto.
	return nil, nil
}

// GetUserRoles returns the role names a user holds for an entity.
// Derived from the module-level permissions returned by GetUserPermissions.
// Format: the proto Permission.Module field is used as a role approximation
// until a dedicated GetUserRoles RPC exists.
func (c *IdentityGRPCClient) GetUserRoles(ctx context.Context, entityID, userID string) ([]string, error) {
	resp, err := c.client.GetUserPermissions(ctx, &identitypb.GetUserPermissionsRequest{
		UserId:   userID,
		EntityId: entityID,
	})
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var roles []string
	for _, perm := range resp.Permissions {
		// Use "MODULE:RESOURCE" as a synthetic role key.
		key := perm.Module + ":" + perm.Resource
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			roles = append(roles, key)
		}
	}
	return roles, nil
}
