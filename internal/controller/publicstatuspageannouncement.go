package controller

import (
	"context"
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/kuehnleon/uptimerobot-operator/api/v1alpha1"
	"github.com/kuehnleon/uptimerobot-operator/internal/uptimerobot"
)

func setupPublicStatusPageAnnouncement(mgr ctrl.Manager, o Options) error {
	r := &syncer[*v1alpha1.PublicStatusPageAnnouncement]{
		Client:   mgr.GetClient(),
		reader:   mgr.GetAPIReader(),
		ur:       o.Client,
		events:   mgr.GetEventRecorder("uptimerobot-operator"),
		interval: o.SyncInterval,
		newObj:   func() *v1alpha1.PublicStatusPageAnnouncement { return &v1alpha1.PublicStatusPageAnnouncement{} },
		pathFor: func(ctx context.Context, a *v1alpha1.PublicStatusPageAnnouncement) (string, error) {
			if !a.DeletionTimestamp.IsZero() && a.Status.PublicStatusPageID != 0 {
				return announcementPath(a.Status.PublicStatusPageID), nil
			}
			var psp v1alpha1.PublicStatusPage
			key := types.NamespacedName{Namespace: a.Namespace, Name: a.Spec.PublicStatusPageRef.Name}
			if err := mgr.GetAPIReader().Get(ctx, key, &psp); err != nil {
				return "", err
			}
			if psp.Status.ID == 0 {
				return "", fmt.Errorf("PublicStatusPage %s is not synced yet", psp.Name)
			}
			a.Status.PublicStatusPageID = psp.Status.ID
			return announcementPath(psp.Status.ID), nil
		},
		path:  "/psps", // pathFor supplies the nested announcement collection.
		drift: []string{"title", "content", "type", "status", "startDate", "endDate"},
		desired: func(_ context.Context, a *v1alpha1.PublicStatusPageAnnouncement) (map[string]any, error) {
			b := map[string]any{
				"title":     a.Spec.Title,
				"content":   a.Spec.Content,
				"type":      or(a.Spec.Type, "Info"),
				"status":    or(a.Spec.Status, "Offline"),
				"startDate": a.Spec.StartDate,
			}
			if a.Spec.EndDate != "" {
				b["endDate"] = a.Spec.EndDate
			}
			return b, nil
		},
		after: func(ctx context.Context, a *v1alpha1.PublicStatusPageAnnouncement, remote uptimerobot.Object) (uptimerobot.Object, error) {
			pageID := a.Status.PublicStatusPageID
			page, err := o.Client.Get(ctx, "/psps", pageID)
			if err != nil {
				return nil, err
			}
			if page == nil {
				return nil, fmt.Errorf("PublicStatusPage UptimeRobot ID %d not found", pageID)
			}
			pinnedID, _ := strconv.ParseInt(fmt.Sprint(page["pinnedAnnouncementId"]), 10, 64)
			if (pinnedID == remote.ID()) == a.Spec.Pinned {
				return remote, nil
			}
			action := "unpin"
			if a.Spec.Pinned {
				action = "pin"
			}
			if _, err := o.Client.Action(ctx, announcementPath(pageID), remote.ID(), action); err != nil {
				return nil, err
			}
			o.Client.Invalidate("/psps")
			return remote, nil
		},
		observe: func(a *v1alpha1.PublicStatusPageAnnouncement, remote uptimerobot.Object) {
			a.Status.RemoteStatus = remote.String("status")
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.PublicStatusPageAnnouncement{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&v1alpha1.PublicStatusPage{}, enqueueAll(mgr.GetClient(), func() client.ObjectList { return &v1alpha1.PublicStatusPageAnnouncementList{} })).
		Complete(r)
}

func announcementPath(pageID int64) string {
	return fmt.Sprintf("/psps/%d/announcements", pageID)
}
