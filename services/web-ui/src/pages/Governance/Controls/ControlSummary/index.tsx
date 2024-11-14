import { Link, useNavigate, useParams } from 'react-router-dom'
import {
    Badge,
    Button,
    Card,
    Divider,
    Flex,
    Grid,
    Icon,
    List,
    ListItem,
    Switch,
    Tab,
    TabGroup,
    TabList,
    TabPanel,
    TabPanels,
    Text,
    Title,
} from '@tremor/react'
import {
    BookOpenIcon,
    ChevronRightIcon,
    ClockIcon,
    CodeBracketIcon,
    CodeBracketSquareIcon,
    Cog8ToothIcon,
    CommandLineIcon,
    DocumentDuplicateIcon,
    InformationCircleIcon,
    PencilIcon,
    Square2StackIcon,
} from '@heroicons/react/24/outline'
import { useSetAtom } from 'jotai/index'
import clipboardCopy from 'clipboard-copy'
import { useState } from 'react'
import Editor from 'react-simple-code-editor'
import { highlight, languages } from 'prismjs'
import Markdown from 'react-markdown'
import MarkdownPreview from '@uiw/react-markdown-preview'
import { useComplianceApiV1ControlsSummaryDetail } from '../../../../api/compliance.gen'
import { notificationAtom, queryAtom } from '../../../../store'
import { severityBadge } from '../index'
import Spinner from '../../../../components/Spinner'
import Detail from './Tabs/Detail'
import ImpactedResources from './Tabs/ImpactedResources'
import Benchmarks from './Tabs/Benchmarks'
import ImpactedAccounts from './Tabs/ImpactedAccounts'
import DrawerPanel from '../../../../components/DrawerPanel'
import { dateTimeDisplay } from '../../../../utilities/dateDisplay'
import TopHeader from '../../../../components/Layout/Header'
import ControlFindings from './Tabs/ControlFindings'
import { useMetadataApiV1QueryParameterList } from '../../../../api/metadata.gen'
import { toErrorMessage } from '../../../../types/apierror'
import { GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus } from '../../../../api/api'
import CodeEditor from '@cloudscape-design/components/code-editor'
import {
    Box,
    BreadcrumbGroup,
    Container,
    CopyToClipboard,
    Header,
    KeyValuePairs,
    SpaceBetween,
    Tabs,
} from '@cloudscape-design/components'
import KGrid from '@cloudscape-design/components/grid'
import SegmentedControl from '@cloudscape-design/components/segmented-control'

export default function ControlDetail() {
    const { controlId, ws } = useParams()
    const setNotification = useSetAtom(notificationAtom)
    const navigate = useNavigate()

    const [doc, setDoc] = useState('')
    const [docTitle, setDocTitle] = useState('')
    const setQuery = useSetAtom(queryAtom)

    const {
        response: controlDetail,
        isLoading,
        error: controlDetailError,
        sendNow: refreshControlDetail,
    } = useComplianceApiV1ControlsSummaryDetail(String(controlId))
    const {
        response: parameters,
        isLoading: parametersLoading,
        isExecuted,
        error: parametersError,
        sendNow: refresh,
    } = useMetadataApiV1QueryParameterList()
    const [conformanceFilter, setConformanceFilter] = useState<
        | GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus[]
        | undefined
    >(undefined)
    const conformanceFilterIdx = () => {
        if (
            conformanceFilter?.length === 1 &&
            conformanceFilter[0] ===
                GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus.ConformanceStatusFailed
        ) {
            return '1'
        }
        if (
            conformanceFilter?.length === 1 &&
            conformanceFilter[0] ===
                GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus.ConformanceStatusPassed
        ) {
            return '2'
        }
        return '0'
    }
    const truncate = (text: string | undefined) => {
        if (text) {
            return text.length > 600 ? text.substring(0, 600) + '...' : text
        }
    }
    const GetBreadCrumb = () => {
        const temp = []
        if (window.location.pathname.includes('incident')) {
            temp.push({
                text: 'Incidents',
                href: `/incidents`,
            })
        } else if (window.location.pathname.includes('compliance')) {
            temp.push({
                text: 'Compliance',
                href: `/compliance`,
            })
        }
        temp.push({ text: 'Control Detail', href: '#' })

        return temp
    }
    const GetKeyValue = () => {
        const temp = [
            {
                label: 'Control ID',
                value: (
                    // @ts-ignore
                    <CopyToClipboard
                        variant="inline"
                        textToCopy={controlDetail?.control?.id || ''}
                        copySuccessText="Control ID copied to clipboard"
                    />
                ),
            },
        ]
        if (controlDetail?.resourceType?.resource_type) {
            temp.push({
                label: 'Resource type',
                value: (
                    // @ts-ignore
                    <CopyToClipboard
                        variant="inline"
                        textToCopy={
                            controlDetail?.resourceType?.resource_type || ''
                        }
                        copySuccessText="Resource type copied to clipboard"
                    />
                ),
            })
        }
        temp.push(
            {
                label: '# of impacted resources',
                value: (
                    // @ts-ignore
                    <>{controlDetail?.totalResourcesCount}</>
                ),
            },
            {
                label: '# of passed resources',
                value: (
                    // @ts-ignore
                    <Text className="text-emerald-500">
                        {(controlDetail?.totalResourcesCount || 0) -
                            (controlDetail?.failedResourcesCount || 0)}
                    </Text>
                ),
            },
            {
                label: '# of failed resources',
                value: (
                    // @ts-ignore
                    <Text className="text-rose-600">
                        {' '}
                        {controlDetail?.failedResourcesCount}
                    </Text>
                ),
            },
            {
                label: 'Last updated',
                value: (
                    // @ts-ignore
                    <>{dateTimeDisplay(controlDetail?.control?.updatedAt)}</>
                ),
            }
        )
        return temp
        // @ts-ignore
    }
    return (
        <>
            {/* <TopHeader
                breadCrumb={[
                    controlDetail
                        ? controlDetail?.control?.title
                        : 'Control detail',
                ]}
            /> */}
            {isLoading ? (
                <Spinner className="mt-56" />
            ) : (
                <>
                    {controlDetail ? (
                        <>
                            <BreadcrumbGroup
                                onClick={(event) => {
                                    // event.preventDefault()
                                }}
                                items={GetBreadCrumb()}
                                ariaLabel="Breadcrumbs"
                            />
                            <Container
                                disableHeaderPaddings
                                disableContentPaddings
                                className="rounded-xl  bg-[#0f2940] p-0 text-white mt-4 "
                                header={
                                    <Header
                                        className={`bg-[#0f2940] p-4 pt-0 rounded-xl   text-white ${
                                            false ? 'rounded-b-none' : ''
                                        }`}
                                        variant="h2"
                                        description=""
                                    >
                                        <SpaceBetween
                                            size="xxxs"
                                            direction="vertical"
                                        >
                                            <Box className="rounded-xl same text-white pt-3 pl-3 pb-0">
                                                <KGrid
                                                    gridDefinition={[
                                                        {
                                                            colspan: {
                                                                default: 12,
                                                                xs: 8,
                                                                s: 9,
                                                            },
                                                        },
                                                        {
                                                            colspan: {
                                                                default: 12,
                                                                xs: 4,
                                                                s: 3,
                                                            },
                                                        },
                                                    ]}
                                                >
                                                    <div>
                                                        <Box
                                                            variant="h1"
                                                            className="text-white important"
                                                        >
                                                            <span className="text-white">
                                                                {
                                                                    controlDetail
                                                                        ?.control
                                                                        ?.title
                                                                }
                                                            </span>
                                                        </Box>
                                                        <Box
                                                            variant="p"
                                                            margin={{
                                                                top: 'xxs',
                                                                bottom: 's',
                                                            }}
                                                        >
                                                            <div className="group text-white important  relative flex text-wrap justify-start">
                                                                <Text className="test-start w-full text-white ">
                                                                    {/* @ts-ignore */}
                                                                    {truncate(
                                                                        controlDetail
                                                                            ?.control
                                                                            ?.description
                                                                    )}
                                                                </Text>
                                                                <Card className="absolute w-full text-wrap z-40 top-0 scale-0 transition-all p-2 group-hover:scale-100">
                                                                    <Text>
                                                                        {
                                                                            controlDetail
                                                                                ?.control
                                                                                ?.description
                                                                        }
                                                                    </Text>
                                                                </Card>
                                                            </div>
                                                        </Box>
                                                    </div>
                                                </KGrid>
                                            </Box>
                                            <Flex className="w-max pl-3">
                                                {' '}
                                                {severityBadge(
                                                    controlDetail?.control
                                                        ?.severity
                                                )}
                                            </Flex>
                                        </SpaceBetween>
                                    </Header>
                                }
                            ></Container>
                            {/* <Flex
                                flexDirection="row"
                                justifyContent="start"
                                className="hover:cursor-pointer max-w-full w-fit bg-gray-200 border-gray-300 rounded-lg border px-1"
                                onClick={() => {
                                    clipboardCopy(
                                        controlDetail?.control?.id || ''
                                    )
                                }}
                            >
                                <Square2StackIcon className="min-w-4 w-4 mr-1" />
                                <Text className="truncate">
                                    Control ID: {controlDetail?.control?.id}
                                </Text>
                            </Flex> */}
                            {/* <Flex
                                flexDirection="row"
                                justifyContent="start"
                                className="max-w-full w-fit bg-gray-200 border-gray-300 rounded-lg border px-1"
                            >
                                <ClockIcon className="min-w-4 w-4 mr-1" />
                                <Text className="truncate">
                                    Last updated:{' '}
                                    {(controlDetail?.evaluatedAt || 0) <= 0
                                        ? 'Never'
                                        : dateTimeDisplay(
                                              controlDetail?.evaluatedAt
                                          )}
                                </Text>
                            </Flex> */}
                            {/* <Flex
                        flexDirection="row"
                        alignItems="start"
                        justifyContent="between"
                        className="mb-6 w-full gap-6"
                    >
                        <Flex
                            flexDirection="col"
                            alignItems="start"
                            justifyContent="start"
                            className="gap-2 w-full"
                        >
                            <Flex className="gap-3 w-fit" alignItems="start">
                                <Title className="font-semibold ">
                                    {controlDetail?.control?.title}
                                </Title>
                                {severityBadge(
                                    controlDetail?.control?.severity
                                )}
                            </Flex>
                            <div className="group  relative flex justify-start">
                                <Text className="">
                                    {controlDetail?.control?.description}
                                </Text>
                                <Card className="absolute w-full z-40 top-0 scale-0 transition-all p-2 group-hover:scale-100">
                                    <Text>
                                        {controlDetail?.control?.description}
                                    </Text>
                                </Card>
                            </div>
                        </Flex>

                        <Flex
                            flexDirection="col"
                            alignItems="end"
                            justifyContent="start"
                            className="w-1/3 gap-2"
                        >
                          

                            {controlDetail?.control?.query?.parameters?.map(
                                (item) => {
                                    return (
                                        <Flex
                                            flexDirection="row"
                                            justifyContent="start"
                                            className="hover:cursor-pointer max-w-full w-fit bg-gray-200 border-gray-300 rounded-lg border px-1"
                                            onClick={() => {
                                                navigate(
                                                    `/compliance/library/parameters?key=${item.key}`
                                                )
                                            }}
                                        >
                                            <PencilIcon className="min-w-4 w-4 mr-1" />
                                            <Text className="truncate">
                                                {item.key}:{' '}
                                                {parameters?.queryParameters
                                                    ?.filter(
                                                        (p) =>
                                                            p.key === item.key
                                                    )
                                                    .map(
                                                        (p) => p.value || ''
                                                    ) || 'Not defined'}
                                            </Text>
                                        </Flex>
                                    )
                                }
                            )}
                        </Flex>
                    </Flex> */}
                            <Grid
                                numItems={2}
                                className=" w-full gap-4 mb-6 mt-4"
                            >
                                <Card className="h-fit min-h-[228px]">
                                    <KeyValuePairs
                                        columns={2}
                                        items={GetKeyValue()}
                                    />
                                    {/* <Flex justifyContent="end">
                                                <Button
                                                    variant="light"
                                                    icon={ChevronRightIcon}
                                                    iconPosition="right"
                                                    onClick={() =>
                                                        setOpenDetail(true)
                                                    }
                                                >
                                                    See more
                                                </Button>
                                            </Flex> */}
                                    {/* <Flex
                                flexDirection="col"
                                alignItems="start"
                                className="h-full"
                            >
                                <List>
                                    <ListItem>
                                        <Text className="whitespace-nowrap mr-2">
                                            Control ID
                                        </Text>
                                        <Flex
                                            flexDirection="row"
                                            className="gap-1 w-full overflow-hidden"
                                            justifyContent="end"
                                            alignItems="end"
                                        >
                                            <Button
                                                variant="light"
                                                onClick={() =>
                                                    clipboardCopy(
                                                        `Control ID: ${controlDetail?.control?.id}`
                                                    ).then(() =>
                                                        setNotification({
                                                            text: 'Control ID copied to clipboard',
                                                            type: 'info',
                                                        })
                                                    )
                                                }
                                                icon={Square2StackIcon}
                                            />
                                            <Text className="text-gray-800 truncate w-fit max-w-full">
                                                {controlDetail?.control?.id}
                                            </Text>
                                        </Flex>
                                    </ListItem>
                                    {controlDetail?.resourceType
                                        ?.resource_type && (
                                        <ListItem>
                                            <Text>Resource type</Text>
                                            <Flex className="gap-1 w-fit">
                                                <Button
                                                    variant="light"
                                                    onClick={() =>
                                                        clipboardCopy(
                                                            `Resource type: ${controlDetail?.resourceType?.resource_type}`
                                                        ).then(() =>
                                                            setNotification({
                                                                text: 'Resource type copied to clipboard',
                                                                type: 'info',
                                                            })
                                                        )
                                                    }
                                                    icon={Square2StackIcon}
                                                />
                                                <Text className="text-gray-800">
                                                    {
                                                        controlDetail
                                                            ?.resourceType
                                                            ?.resource_type
                                                    }
                                                </Text>
                                            </Flex>
                                        </ListItem>
                                    )}

                                    <ListItem>
                                        <Text># of impacted resources</Text>
                                        <Text className="text-gray-800">
                                            {controlDetail?.totalResourcesCount}
                                        </Text>
                                    </ListItem>
                                    <ListItem>
                                        <Text># of passed resources</Text>
                                        <Text className="text-emerald-500">
                                            {(controlDetail?.totalResourcesCount ||
                                                0) -
                                                (controlDetail?.failedResourcesCount ||
                                                    0)}
                                        </Text>
                                    </ListItem>
                                    <ListItem>
                                        <Text># of failed resources</Text>
                                        <Text className="text-rose-600">
                                            {
                                                controlDetail?.failedResourcesCount
                                            }
                                        </Text>
                                    </ListItem>
                                    <ListItem>
                                        <Text>Last updated</Text>
                                        <Text className="text-gray-800">
                                            {dateTimeDisplay(
                                                controlDetail?.control
                                                    ?.updatedAt
                                            )}
                                        </Text>
                                    </ListItem>
                                </List>
                              
                            </Flex> */}
                                </Card>
                                <Card className="max-h-[228px] overflow-scroll">
                                    <Editor
                                        onValueChange={() => 1}
                                        highlight={(text) =>
                                            highlight(
                                                text,
                                                languages.sql,
                                                'sql'
                                            )
                                        }
                                        value={
                                            controlDetail?.control?.query?.queryToExecute?.replace(
                                                '$IS_ALL_CONNECTIONS_QUERY',
                                                'true'
                                            ) || ''
                                        }
                                        className="w-full bg-white dark:bg-gray-900 dark:text-gray-50 font-mono text-sm"
                                        style={{
                                            minHeight: '200px',
                                        }}
                                        placeholder="-- write your SQL query here"
                                    />
                                    <Divider />
                                    <Flex>
                                        <Flex className="gap-4">
                                            <Button
                                                variant="light"
                                                icon={DocumentDuplicateIcon}
                                                iconPosition="left"
                                                onClick={() =>
                                                    clipboardCopy(
                                                        controlDetail?.control?.query?.queryToExecute?.replace(
                                                            '$IS_ALL_CONNECTIONS_QUERY',
                                                            'true'
                                                        ) || ''
                                                    ).then(() =>
                                                        setNotification({
                                                            text: 'Query copied to clipboard',
                                                            type: 'info',
                                                        })
                                                    )
                                                }
                                            >
                                                Copy
                                            </Button>
                                            <Button
                                                variant="secondary"
                                                onClick={() => {
                                                    setQuery(
                                                        controlDetail?.control?.query?.queryToExecute?.replace(
                                                            '$IS_ALL_CONNECTIONS_QUERY',
                                                            'true'
                                                        ) || ''
                                                    )
                                                }}
                                            >
                                                <Link
                                                    to={`/finder?tab_id=1`}
                                                >
                                                    Open in Query
                                                </Link>
                                            </Button>
                                        </Flex>
                                    </Flex>
                                </Card>
                                {/* <TabGroup className="h-full">
                            <TabList
                                variant="solid"
                                className="border border-gray-200 dark:border-gray-700"
                            >
                                <Tab icon={InformationCircleIcon}>
                                    Information
                                </Tab>
                                <Tab icon={CodeBracketSquareIcon}>Query</Tab>
                            </TabList>
                            <TabPanels>
                                <TabPanel className="h-full"></TabPanel>
                                <TabPanel></TabPanel>
                            </TabPanels>
                        </TabGroup> */}
                                <Flex
                                    flexDirection="col"
                                    alignItems="start"
                                    justifyContent="start"
                                    className="h-full"
                                >
                                    <DrawerPanel
                                        title={docTitle}
                                        open={doc.length > 0}
                                        onClose={() => setDoc('')}
                                    >
                                        <MarkdownPreview
                                            source={doc}
                                            className="!bg-transparent"
                                            wrapperElement={{
                                                'data-color-mode': 'light',
                                            }}
                                            rehypeRewrite={(
                                                node,
                                                index,
                                                parent
                                            ) => {
                                                if (
                                                    // eslint-disable-next-line @typescript-eslint/ban-ts-comment
                                                    // @ts-ignore
                                                    node.tagName === 'a' &&
                                                    parent &&
                                                    /^h(1|2|3|4|5|6)/.test(
                                                        // eslint-disable-next-line @typescript-eslint/ban-ts-comment
                                                        // @ts-ignore
                                                        parent.tagName
                                                    )
                                                ) {
                                                    // eslint-disable-next-line no-param-reassign
                                                    parent.children =
                                                        parent.children.slice(1)
                                                }
                                            }}
                                        />
                                    </DrawerPanel>
                                    {/* <Title className="font-semibold mt-2 mb-2">
                                Remediation
                            </Title>
                            <Grid numItems={2} className="w-full h-full gap-4">
                                <Card
                                    className={
                                        controlDetail?.control
                                            ?.manualRemediation &&
                                        controlDetail?.control
                                            ?.manualRemediation.length
                                            ? 'cursor-pointer'
                                            : 'grayscale opacity-70'
                                    }
                                    onClick={() => {
                                        if (
                                            controlDetail?.control
                                                ?.manualRemediation &&
                                            controlDetail?.control
                                                ?.manualRemediation.length
                                        ) {
                                            setDoc(
                                                controlDetail?.control
                                                    ?.manualRemediation
                                            )
                                            setDocTitle('Manual remediation')
                                        }
                                    }}
                                >
                                    <Flex className="mb-2.5">
                                        <Flex
                                            justifyContent="start"
                                            className="w-fit gap-3"
                                        >
                                            <Icon
                                                icon={BookOpenIcon}
                                                className="p-0"
                                            />
                                            <Title className="font-semibold">
                                                Manual
                                            </Title>
                                        </Flex>
                                        <ChevronRightIcon className="w-5 text-openg-500" />
                                    </Flex>
                                    <Text>
                                        Step by Step Guided solution to resolve
                                        instances of non-compliance
                                    </Text>
                                </Card>
                                <Card
                                    className={
                                        controlDetail?.control
                                            ?.cliRemediation &&
                                        controlDetail?.control?.cliRemediation
                                            .length
                                            ? 'cursor-pointer'
                                            : 'grayscale opacity-70'
                                    }
                                    onClick={() => {
                                        if (
                                            controlDetail?.control
                                                ?.cliRemediation &&
                                            controlDetail?.control
                                                ?.cliRemediation.length
                                        ) {
                                            setDoc(
                                                controlDetail?.control
                                                    ?.cliRemediation
                                            )
                                            setDocTitle(
                                                'Command line (CLI) remediation'
                                            )
                                        }
                                    }}
                                >
                                    <Flex className="mb-2.5">
                                        <Flex
                                            justifyContent="start"
                                            className="w-fit gap-3"
                                        >
                                            <Icon
                                                icon={CommandLineIcon}
                                                className="p-0"
                                            />
                                            <Title className="font-semibold">
                                                Command line (CLI)
                                            </Title>
                                        </Flex>
                                        <ChevronRightIcon className="w-5 text-openg-500" />
                                    </Flex>
                                    <Text>
                                        Guided steps to resolve the issue
                                        utilizing CLI
                                    </Text>
                                </Card>
                                <Card
                                    className={
                                        controlDetail?.control
                                            ?.guardrailRemediation &&
                                        controlDetail?.control
                                            ?.guardrailRemediation.length
                                            ? 'cursor-pointer'
                                            : 'grayscale opacity-70'
                                    }
                                    onClick={() => {
                                        if (
                                            controlDetail?.control
                                                ?.guardrailRemediation &&
                                            controlDetail?.control
                                                ?.guardrailRemediation.length
                                        ) {
                                            setDoc(
                                                controlDetail?.control
                                                    ?.guardrailRemediation
                                            )
                                            setDocTitle(
                                                'Guard rails remediation'
                                            )
                                        }
                                    }}
                                >
                                    <Flex className="mb-2.5">
                                        <Flex
                                            justifyContent="start"
                                            className="w-fit gap-3"
                                        >
                                            <Icon
                                                icon={Cog8ToothIcon}
                                                className="p-0"
                                            />
                                            <Title className="font-semibold">
                                                Guard rails
                                            </Title>
                                        </Flex>
                                        <ChevronRightIcon className="w-5 text-openg-500" />
                                    </Flex>
                                    <Text>
                                        Resolve and ensure compliance, at scale
                                        utilizing solutions where possible
                                    </Text>
                                </Card>
                                <Card
                                    className={
                                        controlDetail?.control
                                            ?.programmaticRemediation &&
                                        controlDetail?.control
                                            ?.programmaticRemediation.length
                                            ? 'cursor-pointer'
                                            : 'grayscale opacity-70'
                                    }
                                    onClick={() => {
                                        if (
                                            controlDetail?.control
                                                ?.programmaticRemediation &&
                                            controlDetail?.control
                                                ?.programmaticRemediation.length
                                        ) {
                                            setDoc(
                                                controlDetail?.control
                                                    ?.programmaticRemediation
                                            )
                                            setDocTitle(
                                                'Programmatic remediation'
                                            )
                                        }
                                    }}
                                >
                                    <Flex className="mb-2.5">
                                        <Flex
                                            justifyContent="start"
                                            className="w-fit gap-3"
                                        >
                                            <Icon
                                                icon={CodeBracketIcon}
                                                className="p-0"
                                            />
                                            <Title className="font-semibold">
                                                Programmatic
                                            </Title>
                                        </Flex>
                                        <ChevronRightIcon className="w-5 text-openg-500" />
                                    </Flex>
                                    <Text>
                                        Scripts that help you resolve the issue,
                                        at scale
                                    </Text>
                                </Card>
                            </Grid> */}
                                </Flex>
                            </Grid>
                            <Flex flexDirection="row" className="w-full">
                                <Header
                                    variant="h3"
                                    actions={
                                        <SegmentedControl
                                            selectedId={conformanceFilterIdx()}
                                            onChange={({ detail }) => {
                                                switch (detail.selectedId) {
                                                    case '1':
                                                        setConformanceFilter([
                                                            GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus.ConformanceStatusFailed,
                                                        ])
                                                        break
                                                    case '2':
                                                        setConformanceFilter([
                                                            GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus.ConformanceStatusPassed,
                                                        ])
                                                        break
                                                    default:
                                                        setConformanceFilter(
                                                            undefined
                                                        )
                                                }
                                            }}
                                            label="Default segmented control"
                                            options={[
                                                { text: 'All', id: '0' },
                                                { text: 'Failed', id: '1' },
                                                { text: 'Passed', id: '2' },
                                            ]}
                                        />
                                    }
                                >
                                    Confomance Status filter:
                                </Header>
                                {/* <Text className="mr-2 w-fit">
                            Confomance Status filter:
                        </Text>
                        <SegmentedControl
                            selectedId={conformanceFilterIdx()}
                            onChange={({ detail }) => {
                                switch (detail.selectedId) {
                                    case '1':
                                        setConformanceFilter([
                                            GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus.ConformanceStatusFailed,
                                        ])
                                        break
                                    case '2':
                                        setConformanceFilter([
                                            GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus.ConformanceStatusPassed,
                                        ])
                                        break
                                    default:
                                        setConformanceFilter(undefined)
                                }
                            }}
                            label="Default segmented control"
                            options={[
                                { text: 'All', id: '0' },
                                { text: 'Failed', id: '1' },
                                { text: 'Passes', id: '2' },
                            ]}
                        /> */}
                                {/* <TabGroup
                            tabIndex={conformanceFilterIdx()}
                            className="w-fit"
                            onIndexChange={(tabIndex) => {
                                switch (tabIndex) {
                                    case 1:
                                        setConformanceFilter([
                                            GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus.ConformanceStatusFailed,
                                        ])
                                        break
                                    case 2:
                                        setConformanceFilter([
                                            GithubComKaytuIoKaytuEnginePkgComplianceApiConformanceStatus.ConformanceStatusPassed,
                                        ])
                                        break
                                    default:
                                        setConformanceFilter(undefined)
                                }
                            }}
                        >
                            <TabList variant="solid">
                                <Tab value="1">All</Tab>
                                <Tab value="2">Failed</Tab>
                                <Tab value="3">Passed</Tab>
                            </TabList>
                        </TabGroup> */}
                            </Flex>
                            <Tabs
                                tabs={[
                                    {
                                        label: 'Impacted resources',
                                        id: '0',
                                        content: (
                                            <ImpactedResources
                                                controlId={
                                                    controlDetail?.control
                                                        ?.id || ''
                                                }
                                                linkPrefix={`/score/categories/`}
                                                conformanceFilter={
                                                    conformanceFilter
                                                }
                                            />
                                        ),
                                    },
                                    {
                                        id: '1',
                                        label: 'Impacted accounts',
                                        content: (
                                            <ImpactedAccounts
                                                controlId={
                                                    controlDetail?.control?.id
                                                }
                                            />
                                        ),
                                    },
                                    // {
                                    //     id: '2',
                                    //     label: 'Control information',
                                    //     content: (
                                    //         <Detail control={controlDetail?.control} />
                                    //     ),
                                    //     disabled:
                                    //         controlDetail?.control?.explanation
                                    //             ?.length === 0 &&
                                    //         controlDetail?.control?.nonComplianceCost
                                    //             ?.length === 0 &&
                                    //         controlDetail?.control?.usefulExample
                                    //             ?.length === 0,
                                    //     disabledReason: 'Control has no explanation',
                                    // },
                                    {
                                        id: '3',
                                        label: 'Benchmarks',
                                        content: (
                                            <Benchmarks
                                                benchmarks={
                                                    controlDetail?.benchmarks
                                                }
                                            />
                                        ),
                                    },
                                    // {
                                    //     id: '4',
                                    //     label: 'Incidents',
                                    //     content: (
                                    //         <ControlFindings
                                    //             controlId={controlDetail?.control?.id}
                                    //         />
                                    //     ),
                                    // },
                                ]}
                            />
                            {/* <TabGroup>
                        <Flex
                            flexDirection="row"
                            justifyContent="between"
                            className="mb-2"
                        >
                            <div className="w-fit">
                                <TabList>
                                    <Tab>Impacted resources</Tab>
                                    <Tab>Impacted accounts</Tab>
                                    <Tab
                                        disabled={
                                            controlDetail?.control?.explanation
                                                ?.length === 0 &&
                                            controlDetail?.control
                                                ?.nonComplianceCost?.length ===
                                                0 &&
                                            controlDetail?.control
                                                ?.usefulExample?.length === 0
                                        }
                                    >
                                        Control information
                                    </Tab>
                                    <Tab>Benchmarks</Tab>
                                    <Tab>Findings</Tab>
                                </TabList>
                            </div>
                        </Flex>
                        <TabPanels>
                            <TabPanel>
                                <ImpactedResources
                                    controlId={controlDetail?.control?.id || ''}
                                    linkPrefix={`/score/categories/`}
                                    conformanceFilter={conformanceFilter}
                                />
                            </TabPanel>
                            <TabPanel>
                                <ImpactedAccounts
                                    controlId={controlDetail?.control?.id}
                                />
                            </TabPanel>
                            <TabPanel>
                                <Detail control={controlDetail?.control} />
                            </TabPanel>
                            <TabPanel>
                                <Benchmarks
                                    benchmarks={controlDetail?.benchmarks}
                                />
                            </TabPanel>
                            <TabPanel>
                                <ControlFindings
                                    controlId={controlDetail?.control?.id}
                                />
                            </TabPanel>
                        </TabPanels>
                    </TabGroup> */}
                            {toErrorMessage(controlDetailError) && (
                                <Flex
                                    flexDirection="col"
                                    justifyContent="between"
                                    className="absolute top-0 w-full left-0 h-full backdrop-blur"
                                >
                                    <Flex
                                        flexDirection="col"
                                        justifyContent="center"
                                        alignItems="center"
                                    >
                                        <Title className="mt-6">
                                            Failed to load component
                                        </Title>
                                        <Text className="mt-2">
                                            {toErrorMessage(controlDetailError)}
                                        </Text>
                                    </Flex>
                                    <Button
                                        variant="secondary"
                                        className="mb-6"
                                        color="slate"
                                        onClick={() => {
                                            refreshControlDetail()
                                        }}
                                    >
                                        Try Again
                                    </Button>
                                </Flex>
                            )}
                        </>
                    ) : (
                        <>
                            {toErrorMessage(controlDetailError) && (
                                <Flex
                                    flexDirection="col"
                                    justifyContent="between"
                                    className="absolute top-0 w-full left-0 h-full backdrop-blur"
                                >
                                    <Flex
                                        flexDirection="col"
                                        justifyContent="center"
                                        alignItems="center"
                                    >
                                        <Title className="mt-6">
                                            Failed to load component
                                        </Title>
                                        <Text className="mt-2">
                                            {toErrorMessage(controlDetailError)}
                                        </Text>
                                    </Flex>
                                    <Button
                                        variant="secondary"
                                        className="mb-6"
                                        color="slate"
                                        onClick={() => {
                                            refreshControlDetail()
                                        }}
                                    >
                                        Try Again
                                    </Button>
                                </Flex>
                            )}
                        </>
                    )}
                </>
            )}
        </>
    )
}
