package v1beta1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	barbicanv1 "github.com/openstack-k8s-operators/barbican-operator/api/v1beta1"
	cinderv1 "github.com/openstack-k8s-operators/cinder-operator/api/v1beta1"
	designatev1 "github.com/openstack-k8s-operators/designate-operator/api/v1beta1"
	heatv1 "github.com/openstack-k8s-operators/heat-operator/api/v1beta1"
	rabbitmqv1 "github.com/openstack-k8s-operators/infra-operator/apis/rabbitmq/v1beta1"
	ironicv1 "github.com/openstack-k8s-operators/ironic-operator/api/v1beta1"
	keystonev1 "github.com/openstack-k8s-operators/keystone-operator/api/v1beta1"
	manilav1 "github.com/openstack-k8s-operators/manila-operator/api/v1beta1"
	neutronv1 "github.com/openstack-k8s-operators/neutron-operator/api/v1beta1"
	novav1 "github.com/openstack-k8s-operators/nova-operator/api/v1beta1"
	octaviav1 "github.com/openstack-k8s-operators/octavia-operator/api/v1beta1"
	swiftv1 "github.com/openstack-k8s-operators/swift-operator/api/v1beta1"
	telemetryv1 "github.com/openstack-k8s-operators/telemetry-operator/api/v1beta1"
	watcherv1 "github.com/openstack-k8s-operators/watcher-operator/api/v1beta1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

var _ = Describe("OpenStackControlPlane Webhook", func() {

	Context("ValidateMessagingBusConfig", func() {
		var instance *OpenStackControlPlane
		var basePath *field.Path

		BeforeEach(func() {
			instance = &OpenStackControlPlane{
				Spec: OpenStackControlPlaneSpec{},
			}
			basePath = field.NewPath("spec")
		})

		It("should allow only Cluster field in messagingBus", func() {
			instance.Spec.MessagingBus = &rabbitmqv1.RabbitMqConfig{
				Cluster: "rabbitmq",
			}

			errs := instance.ValidateMessagingBusConfig(basePath)
			Expect(errs).To(BeEmpty())
		})

		It("should allow Cluster and Vhost fields in messagingBus", func() {
			instance.Spec.MessagingBus = &rabbitmqv1.RabbitMqConfig{
				Cluster: "rabbitmq",
				Vhost:   "/openstack",
			}

			errs := instance.ValidateMessagingBusConfig(basePath)
			Expect(errs).To(BeEmpty())
		})

		It("should reject User field in messagingBus", func() {
			instance.Spec.MessagingBus = &rabbitmqv1.RabbitMqConfig{
				Cluster: "rabbitmq",
				User:    "shared-user",
			}

			errs := instance.ValidateMessagingBusConfig(basePath)
			Expect(errs).To(HaveLen(1))
			Expect(errs[0].Type).To(Equal(field.ErrorTypeForbidden))
			Expect(errs[0].Field).To(Equal("spec.messagingBus.user"))
			Expect(errs[0].Detail).To(ContainSubstring("user field is not allowed at the top level"))
		})

		It("should reject User field even with other valid fields in messagingBus", func() {
			instance.Spec.MessagingBus = &rabbitmqv1.RabbitMqConfig{
				Cluster: "rabbitmq",
				Vhost:   "/openstack",
				User:    "shared-user",
			}

			errs := instance.ValidateMessagingBusConfig(basePath)
			Expect(errs).To(HaveLen(1))
			Expect(errs[0].Type).To(Equal(field.ErrorTypeForbidden))
			Expect(errs[0].Field).To(Equal("spec.messagingBus.user"))
		})

		It("should allow only Cluster field in notificationsBus", func() {
			instance.Spec.NotificationsBus = &rabbitmqv1.RabbitMqConfig{
				Cluster: "rabbitmq-notifications",
			}

			errs := instance.ValidateMessagingBusConfig(basePath)
			Expect(errs).To(BeEmpty())
		})

		It("should allow Cluster and Vhost fields in notificationsBus", func() {
			instance.Spec.NotificationsBus = &rabbitmqv1.RabbitMqConfig{
				Cluster: "rabbitmq-notifications",
				Vhost:   "/notifications",
			}

			errs := instance.ValidateMessagingBusConfig(basePath)
			Expect(errs).To(BeEmpty())
		})

		It("should reject User field in notificationsBus", func() {
			instance.Spec.NotificationsBus = &rabbitmqv1.RabbitMqConfig{
				Cluster: "rabbitmq-notifications",
				User:    "shared-user",
			}

			errs := instance.ValidateMessagingBusConfig(basePath)
			Expect(errs).To(HaveLen(1))
			Expect(errs[0].Type).To(Equal(field.ErrorTypeForbidden))
			Expect(errs[0].Field).To(Equal("spec.notificationsBus.user"))
			Expect(errs[0].Detail).To(ContainSubstring("user field is not allowed at the top level"))
		})

		It("should reject User field in both messagingBus and notificationsBus", func() {
			instance.Spec.MessagingBus = &rabbitmqv1.RabbitMqConfig{
				Cluster: "rabbitmq",
				User:    "rpc-user",
			}
			instance.Spec.NotificationsBus = &rabbitmqv1.RabbitMqConfig{
				Cluster: "rabbitmq-notifications",
				User:    "notif-user",
			}

			errs := instance.ValidateMessagingBusConfig(basePath)
			Expect(errs).To(HaveLen(2))
			Expect(errs[0].Field).To(Equal("spec.messagingBus.user"))
			Expect(errs[1].Field).To(Equal("spec.notificationsBus.user"))
		})

		It("should allow nil messagingBus and notificationsBus", func() {
			instance.Spec.MessagingBus = nil
			instance.Spec.NotificationsBus = nil

			errs := instance.ValidateMessagingBusConfig(basePath)
			Expect(errs).To(BeEmpty())
		})
	})

	Context("migrateDeprecatedFields", func() {
		var instance *OpenStackControlPlane

		BeforeEach(func() {
			instance = &OpenStackControlPlane{
				Spec: OpenStackControlPlaneSpec{},
			}
		})

		It("should migrate NotificationsBusInstance to NotificationsBus.Cluster", func() {
			deprecatedValue := "rabbitmq-notifications"
			instance.Spec.NotificationsBusInstance = &deprecatedValue

			instance.migrateDeprecatedFields()

			Expect(instance.Spec.NotificationsBus).ToNot(BeNil())
			Expect(instance.Spec.NotificationsBus.Cluster).To(Equal("rabbitmq-notifications"))
			Expect(instance.Spec.NotificationsBusInstance).To(BeNil())
		})

		It("should not overwrite existing NotificationsBus.Cluster", func() {
			deprecatedValue := "rabbitmq-old"
			instance.Spec.NotificationsBusInstance = &deprecatedValue
			instance.Spec.NotificationsBus = &rabbitmqv1.RabbitMqConfig{
				Cluster: "rabbitmq-new",
			}

			instance.migrateDeprecatedFields()

			Expect(instance.Spec.NotificationsBus.Cluster).To(Equal("rabbitmq-new"))
			Expect(instance.Spec.NotificationsBusInstance).To(BeNil())
		})

		It("should handle empty NotificationsBusInstance", func() {
			emptyValue := ""
			instance.Spec.NotificationsBusInstance = &emptyValue

			instance.migrateDeprecatedFields()

			// Should not create NotificationsBus for empty deprecated value
			Expect(instance.Spec.NotificationsBus).To(BeNil())
			Expect(instance.Spec.NotificationsBusInstance).To(Equal(&emptyValue))
		})

		It("should handle nil NotificationsBusInstance", func() {
			instance.Spec.NotificationsBusInstance = nil

			instance.migrateDeprecatedFields()

			Expect(instance.Spec.NotificationsBus).To(BeNil())
			Expect(instance.Spec.NotificationsBusInstance).To(BeNil())
		})

		It("should preserve other NotificationsBus fields during migration", func() {
			deprecatedValue := "rabbitmq-cluster"
			instance.Spec.NotificationsBusInstance = &deprecatedValue
			instance.Spec.NotificationsBus = &rabbitmqv1.RabbitMqConfig{
				Vhost: "/custom-vhost",
			}

			instance.migrateDeprecatedFields()

			Expect(instance.Spec.NotificationsBus.Cluster).To(Equal("rabbitmq-cluster"))
			Expect(instance.Spec.NotificationsBus.Vhost).To(Equal("/custom-vhost"))
			Expect(instance.Spec.NotificationsBusInstance).To(BeNil())
		})
	})

	Context("ValidateUpdate with annotation trigger", func() {
		const reconcileTriggerAnnotation = "openstack.org/reconcile-trigger"

		It("should perform migration when reconcile trigger annotation is present", func() {
			deprecatedValue := "rabbitmq-notifications"
			instance := &OpenStackControlPlane{
				Spec: OpenStackControlPlaneSpec{
					NotificationsBusInstance: &deprecatedValue,
				},
			}
			instance.SetAnnotations(map[string]string{
				reconcileTriggerAnnotation: "2024-01-01T00:00:00Z",
			})

			oldInstance := &OpenStackControlPlane{
				Spec: OpenStackControlPlaneSpec{
					NotificationsBusInstance: &deprecatedValue,
				},
			}

			_, err := instance.ValidateUpdate(nil, oldInstance, nil)
			Expect(err).ToNot(HaveOccurred())

			// Migration should have occurred
			Expect(instance.Spec.NotificationsBus).ToNot(BeNil())
			Expect(instance.Spec.NotificationsBus.Cluster).To(Equal("rabbitmq-notifications"))
			Expect(instance.Spec.NotificationsBusInstance).To(BeNil())

			// Annotation should be removed
			Expect(instance.GetAnnotations()).ToNot(HaveKey(reconcileTriggerAnnotation))
		})

		It("should not perform migration when annotation is absent", func() {
			deprecatedValue := "rabbitmq-notifications"
			instance := &OpenStackControlPlane{
				Spec: OpenStackControlPlaneSpec{
					NotificationsBusInstance: &deprecatedValue,
				},
			}

			oldInstance := &OpenStackControlPlane{
				Spec: OpenStackControlPlaneSpec{
					NotificationsBusInstance: &deprecatedValue,
				},
			}

			_, err := instance.ValidateUpdate(nil, oldInstance, nil)
			Expect(err).ToNot(HaveOccurred())

			// Migration should NOT have occurred
			Expect(instance.Spec.NotificationsBus).To(BeNil())
			Expect(instance.Spec.NotificationsBusInstance).To(Equal(&deprecatedValue))
		})
	})

	Context("Service-level messaging bus migrations", func() {
		var instance *OpenStackControlPlane

		BeforeEach(func() {
			instance = &OpenStackControlPlane{
				Spec: OpenStackControlPlaneSpec{},
			}
		})

		It("should migrate all service-level rabbitMqClusterName to messagingBus.cluster", func() {
			// Cinder
			cinderTemplate := &cinderv1.CinderSpecCore{}
			cinderTemplate.RabbitMqClusterName = "cinder-rmq"
			instance.Spec.Cinder.Template = cinderTemplate

			// Manila
			manilaTemplate := &manilav1.ManilaSpecCore{}
			manilaTemplate.RabbitMqClusterName = "manila-rmq"
			instance.Spec.Manila.Template = manilaTemplate

			// Neutron
			neutronTemplate := &neutronv1.NeutronAPISpecCore{}
			neutronTemplate.RabbitMqClusterName = "neutron-rmq"
			instance.Spec.Neutron.Template = neutronTemplate

			// Heat
			heatTemplate := &heatv1.HeatSpecCore{}
			heatTemplate.RabbitMqClusterName = "heat-rmq"
			instance.Spec.Heat.Template = heatTemplate

			// Ironic
			ironicTemplate := &ironicv1.IronicSpecCore{}
			ironicTemplate.RabbitMqClusterName = "ironic-rmq"
			instance.Spec.Ironic.Template = ironicTemplate

			// Barbican
			barbicanTemplate := &barbicanv1.BarbicanSpecCore{}
			barbicanTemplate.RabbitMqClusterName = "barbican-rmq"
			instance.Spec.Barbican.Template = barbicanTemplate

			// Designate
			designateTemplate := &designatev1.DesignateSpecCore{}
			designateTemplate.RabbitMqClusterName = "designate-rmq"
			instance.Spec.Designate.Template = designateTemplate

			// Octavia
			octaviaTemplate := &octaviav1.OctaviaSpecCore{}
			octaviaTemplate.RabbitMqClusterName = "octavia-rmq"
			instance.Spec.Octavia.Template = octaviaTemplate

			// Watcher (uses pointer for RabbitMqClusterName)
			watcherTemplate := &watcherv1.WatcherSpecCore{}
			watcherRmq := "watcher-rmq"
			watcherTemplate.RabbitMqClusterName = &watcherRmq
			instance.Spec.Watcher.Template = watcherTemplate

			// Execute migration
			instance.migrateDeprecatedFields()

			// Verify: All services migrated and deprecated fields cleared
			Expect(instance.Spec.Cinder.Template.MessagingBus.Cluster).To(Equal("cinder-rmq"))
			Expect(instance.Spec.Cinder.Template.RabbitMqClusterName).To(Equal(""))

			Expect(instance.Spec.Manila.Template.MessagingBus.Cluster).To(Equal("manila-rmq"))
			Expect(instance.Spec.Manila.Template.RabbitMqClusterName).To(Equal(""))

			Expect(instance.Spec.Neutron.Template.MessagingBus.Cluster).To(Equal("neutron-rmq"))
			Expect(instance.Spec.Neutron.Template.RabbitMqClusterName).To(Equal(""))

			Expect(instance.Spec.Heat.Template.MessagingBus.Cluster).To(Equal("heat-rmq"))
			Expect(instance.Spec.Heat.Template.RabbitMqClusterName).To(Equal(""))

			Expect(instance.Spec.Ironic.Template.MessagingBus.Cluster).To(Equal("ironic-rmq"))
			Expect(instance.Spec.Ironic.Template.RabbitMqClusterName).To(Equal(""))

			Expect(instance.Spec.Barbican.Template.MessagingBus.Cluster).To(Equal("barbican-rmq"))
			Expect(instance.Spec.Barbican.Template.RabbitMqClusterName).To(Equal(""))

			Expect(instance.Spec.Designate.Template.MessagingBus.Cluster).To(Equal("designate-rmq"))
			Expect(instance.Spec.Designate.Template.RabbitMqClusterName).To(Equal(""))

			Expect(instance.Spec.Octavia.Template.MessagingBus.Cluster).To(Equal("octavia-rmq"))
			Expect(instance.Spec.Octavia.Template.RabbitMqClusterName).To(Equal(""))

			Expect(instance.Spec.Watcher.Template.MessagingBus.Cluster).To(Equal("watcher-rmq"))
			Expect(instance.Spec.Watcher.Template.RabbitMqClusterName).To(BeNil())
		})

		It("should migrate Keystone rabbitMqClusterName to notificationsBus.cluster", func() {
			keystoneTemplate := &keystonev1.KeystoneAPISpecCore{}
			keystoneTemplate.RabbitMqClusterName = "keystone-rmq"
			instance.Spec.Keystone.Template = keystoneTemplate

			instance.migrateDeprecatedFields()

			Expect(instance.Spec.Keystone.Template.NotificationsBus).ToNot(BeNil())
			Expect(instance.Spec.Keystone.Template.NotificationsBus.Cluster).To(Equal("keystone-rmq"))
			Expect(instance.Spec.Keystone.Template.RabbitMqClusterName).To(Equal(""))
		})

		It("should migrate Swift SwiftProxy rabbitMqClusterName to notificationsBus.cluster", func() {
			swiftTemplate := &swiftv1.SwiftSpecCore{}
			swiftTemplate.SwiftProxy.RabbitMqClusterName = "swift-rmq"
			instance.Spec.Swift.Template = swiftTemplate

			instance.migrateDeprecatedFields()

			Expect(instance.Spec.Swift.Template.SwiftProxy.NotificationsBus).ToNot(BeNil())
			Expect(instance.Spec.Swift.Template.SwiftProxy.NotificationsBus.Cluster).To(Equal("swift-rmq"))
			Expect(instance.Spec.Swift.Template.SwiftProxy.RabbitMqClusterName).To(Equal(""))
		})

		It("should migrate Nova API and cell-level messaging bus fields", func() {
			novaTemplate := &novav1.NovaSpecCore{}
			novaTemplate.APIMessageBusInstance = "nova-api-rmq"
			novaTemplate.CellTemplates = map[string]novav1.NovaCellTemplate{
				"cell0": {
					CellMessageBusInstance: "nova-cell0-rmq",
				},
				"cell1": {
					CellMessageBusInstance: "nova-cell1-rmq",
				},
			}
			instance.Spec.Nova.Template = novaTemplate

			instance.migrateDeprecatedFields()

			// Verify API-level migration
			Expect(instance.Spec.Nova.Template.MessagingBus.Cluster).To(Equal("nova-api-rmq"))
			Expect(instance.Spec.Nova.Template.APIMessageBusInstance).To(Equal(""))

			// Verify cell-level migrations
			Expect(instance.Spec.Nova.Template.CellTemplates["cell0"].MessagingBus.Cluster).To(Equal("nova-cell0-rmq"))
			Expect(instance.Spec.Nova.Template.CellTemplates["cell0"].CellMessageBusInstance).To(Equal(""))

			Expect(instance.Spec.Nova.Template.CellTemplates["cell1"].MessagingBus.Cluster).To(Equal("nova-cell1-rmq"))
			Expect(instance.Spec.Nova.Template.CellTemplates["cell1"].CellMessageBusInstance).To(Equal(""))
		})

		It("should migrate Telemetry sub-services correctly", func() {
			telemetryTemplate := &telemetryv1.TelemetrySpecCore{}

			// CloudKitty - uses MessagingBus
			telemetryTemplate.CloudKitty.RabbitMqClusterName = "cloudkitty-rmq"

			// Aodh - uses NotificationsBus
			telemetryTemplate.Autoscaling.Aodh.RabbitMqClusterName = "aodh-rmq"

			// Ceilometer - uses NotificationsBus
			telemetryTemplate.Ceilometer.RabbitMqClusterName = "ceilometer-rmq"

			instance.Spec.Telemetry.Template = telemetryTemplate

			instance.migrateDeprecatedFields()

			// CloudKitty uses MessagingBus
			Expect(instance.Spec.Telemetry.Template.CloudKitty.MessagingBus.Cluster).To(Equal("cloudkitty-rmq"))
			Expect(instance.Spec.Telemetry.Template.CloudKitty.RabbitMqClusterName).To(Equal(""))

			// Aodh uses NotificationsBus
			Expect(instance.Spec.Telemetry.Template.Autoscaling.Aodh.NotificationsBus).ToNot(BeNil())
			Expect(instance.Spec.Telemetry.Template.Autoscaling.Aodh.NotificationsBus.Cluster).To(Equal("aodh-rmq"))
			Expect(instance.Spec.Telemetry.Template.Autoscaling.Aodh.RabbitMqClusterName).To(Equal(""))

			// Ceilometer uses NotificationsBus
			Expect(instance.Spec.Telemetry.Template.Ceilometer.NotificationsBus).ToNot(BeNil())
			Expect(instance.Spec.Telemetry.Template.Ceilometer.NotificationsBus.Cluster).To(Equal("ceilometer-rmq"))
			Expect(instance.Spec.Telemetry.Template.Ceilometer.RabbitMqClusterName).To(Equal(""))
		})

		It("should migrate IronicNeutronAgent with parent-service inheritance", func() {
			ironicTemplate := &ironicv1.IronicSpecCore{}
			ironicTemplate.RabbitMqClusterName = "ironic-rmq"
			ironicTemplate.IronicNeutronAgent.RabbitMqClusterName = "ironic-agent-rmq"
			instance.Spec.Ironic.Template = ironicTemplate

			instance.migrateDeprecatedFields()

			// Verify Ironic main template migrated
			Expect(instance.Spec.Ironic.Template.MessagingBus.Cluster).To(Equal("ironic-rmq"))
			Expect(instance.Spec.Ironic.Template.RabbitMqClusterName).To(Equal(""))

			// Verify IronicNeutronAgent migrated
			Expect(instance.Spec.Ironic.Template.IronicNeutronAgent.MessagingBus.Cluster).To(Equal("ironic-agent-rmq"))
			Expect(instance.Spec.Ironic.Template.IronicNeutronAgent.RabbitMqClusterName).To(Equal(""))
		})

		It("should inherit from parent service when IronicNeutronAgent deprecated field is empty", func() {
			ironicTemplate := &ironicv1.IronicSpecCore{}
			ironicTemplate.RabbitMqClusterName = "ironic-rmq"
			// IronicNeutronAgent.RabbitMqClusterName is not set - should inherit from parent
			instance.Spec.Ironic.Template = ironicTemplate

			instance.migrateDeprecatedFields()

			// Ironic migrated
			Expect(instance.Spec.Ironic.Template.MessagingBus.Cluster).To(Equal("ironic-rmq"))

			// IronicNeutronAgent inherited from parent (Ironic)
			Expect(instance.Spec.Ironic.Template.IronicNeutronAgent.MessagingBus.Cluster).To(Equal("ironic-rmq"))
		})

		It("should inherit from top-level messagingBus when service deprecated field is empty", func() {
			instance.Spec.MessagingBus = &rabbitmqv1.RabbitMqConfig{
				Cluster: "top-level-rmq",
				Vhost:   "/custom",
			}
			cinderTemplate := &cinderv1.CinderSpecCore{}
			// No RabbitMqClusterName set - should inherit from top-level
			instance.Spec.Cinder.Template = cinderTemplate

			instance.migrateDeprecatedFields()

			Expect(instance.Spec.Cinder.Template.MessagingBus.Cluster).To(Equal("top-level-rmq"))
			Expect(instance.Spec.Cinder.Template.MessagingBus.Vhost).To(Equal("/custom"))
		})

		It("should inherit from top-level notificationsBus for Keystone when deprecated field is empty", func() {
			instance.Spec.NotificationsBus = &rabbitmqv1.RabbitMqConfig{
				Cluster: "top-level-notifications",
			}
			keystoneTemplate := &keystonev1.KeystoneAPISpecCore{}
			// No RabbitMqClusterName set - should inherit from top-level
			instance.Spec.Keystone.Template = keystoneTemplate

			instance.migrateDeprecatedFields()

			Expect(instance.Spec.Keystone.Template.NotificationsBus).ToNot(BeNil())
			Expect(instance.Spec.Keystone.Template.NotificationsBus.Cluster).To(Equal("top-level-notifications"))
		})

		It("should apply default 'rabbitmq' when no deprecated field and no top-level", func() {
			cinderTemplate := &cinderv1.CinderSpecCore{}
			// No RabbitMqClusterName, no top-level - should default
			instance.Spec.Cinder.Template = cinderTemplate

			instance.migrateDeprecatedFields()

			Expect(instance.Spec.Cinder.Template.MessagingBus.Cluster).To(Equal("rabbitmq"))
		})

		It("should NOT default notificationsBus (optional field)", func() {
			keystoneTemplate := &keystonev1.KeystoneAPISpecCore{}
			// No RabbitMqClusterName, no top-level - should remain nil
			instance.Spec.Keystone.Template = keystoneTemplate

			instance.migrateDeprecatedFields()

			// NotificationsBus is optional, so it should remain nil
			Expect(instance.Spec.Keystone.Template.NotificationsBus).To(BeNil())
		})

		It("should migrate all service-level notificationsBusInstance to notificationsBus.cluster", func() {
			// Cinder
			cinderTemplate := &cinderv1.CinderSpecCore{}
			cinderNotif := "cinder-notif-rmq"
			cinderTemplate.NotificationsBusInstance = &cinderNotif
			instance.Spec.Cinder.Template = cinderTemplate

			// Manila
			manilaTemplate := &manilav1.ManilaSpecCore{}
			manilaNotif := "manila-notif-rmq"
			manilaTemplate.NotificationsBusInstance = &manilaNotif
			instance.Spec.Manila.Template = manilaTemplate

			// Neutron
			neutronTemplate := &neutronv1.NeutronAPISpecCore{}
			neutronNotif := "neutron-notif-rmq"
			neutronTemplate.NotificationsBusInstance = &neutronNotif
			instance.Spec.Neutron.Template = neutronTemplate

			// Nova
			novaTemplate := &novav1.NovaSpecCore{}
			novaNotif := "nova-notif-rmq"
			novaTemplate.NotificationsBusInstance = &novaNotif
			instance.Spec.Nova.Template = novaTemplate

			// Watcher
			watcherTemplate := &watcherv1.WatcherSpecCore{}
			watcherNotif := "watcher-notif-rmq"
			watcherTemplate.NotificationsBusInstance = &watcherNotif
			instance.Spec.Watcher.Template = watcherTemplate

			// Execute migration
			instance.migrateDeprecatedFields()

			// Verify: All services migrated and deprecated fields cleared
			Expect(instance.Spec.Cinder.Template.NotificationsBus).ToNot(BeNil())
			Expect(instance.Spec.Cinder.Template.NotificationsBus.Cluster).To(Equal("cinder-notif-rmq"))
			Expect(instance.Spec.Cinder.Template.NotificationsBusInstance).To(BeNil())

			Expect(instance.Spec.Manila.Template.NotificationsBus).ToNot(BeNil())
			Expect(instance.Spec.Manila.Template.NotificationsBus.Cluster).To(Equal("manila-notif-rmq"))
			Expect(instance.Spec.Manila.Template.NotificationsBusInstance).To(BeNil())

			Expect(instance.Spec.Neutron.Template.NotificationsBus).ToNot(BeNil())
			Expect(instance.Spec.Neutron.Template.NotificationsBus.Cluster).To(Equal("neutron-notif-rmq"))
			Expect(instance.Spec.Neutron.Template.NotificationsBusInstance).To(BeNil())

			Expect(instance.Spec.Nova.Template.NotificationsBus).ToNot(BeNil())
			Expect(instance.Spec.Nova.Template.NotificationsBus.Cluster).To(Equal("nova-notif-rmq"))
			Expect(instance.Spec.Nova.Template.NotificationsBusInstance).To(BeNil())

			Expect(instance.Spec.Watcher.Template.NotificationsBus).ToNot(BeNil())
			Expect(instance.Spec.Watcher.Template.NotificationsBus.Cluster).To(Equal("watcher-notif-rmq"))
			Expect(instance.Spec.Watcher.Template.NotificationsBusInstance).To(BeNil())
		})

		It("should not overwrite existing service-level notificationsBus.cluster", func() {
			// Cinder - has deprecated field but also has new field set
			cinderTemplate := &cinderv1.CinderSpecCore{}
			cinderDeprecated := "cinder-old"
			cinderTemplate.NotificationsBusInstance = &cinderDeprecated
			cinderTemplate.NotificationsBus = &rabbitmqv1.RabbitMqConfig{
				Cluster: "cinder-new",
			}
			instance.Spec.Cinder.Template = cinderTemplate

			instance.migrateDeprecatedFields()

			// Should keep the existing value, not overwrite
			Expect(instance.Spec.Cinder.Template.NotificationsBus.Cluster).To(Equal("cinder-new"))
			Expect(instance.Spec.Cinder.Template.NotificationsBusInstance).To(BeNil())
		})

		It("should inherit top-level notificationsBus when service-level is nil", func() {
			// Set top-level notificationsBus
			instance.Spec.NotificationsBus = &rabbitmqv1.RabbitMqConfig{
				Cluster: "top-level-notif",
			}

			// Cinder has no notificationsBusInstance and no notificationsBus
			cinderTemplate := &cinderv1.CinderSpecCore{}
			instance.Spec.Cinder.Template = cinderTemplate

			instance.migrateDeprecatedFields()

			// Should inherit from top-level
			Expect(instance.Spec.Cinder.Template.NotificationsBus).ToNot(BeNil())
			Expect(instance.Spec.Cinder.Template.NotificationsBus.Cluster).To(Equal("top-level-notif"))
		})
	})
})
